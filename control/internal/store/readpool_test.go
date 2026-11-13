package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestReadOnlySQL(t *testing.T) {
	cases := []struct {
		q    string
		read bool
	}{
		{`SELECT 1`, true},
		{`select id from users`, true},
		{"  \n\tSELECT COUNT(*) FROM users", true},
		{"-- comment\nSELECT 1", true},
		{"/* c */ /* d */ select 1", true},
		{`WITH x AS (SELECT 1) SELECT * FROM x`, true},
		{`with recent as (select id from audit_log) select count(*) from recent`, true},
		{`SELECT updated_at, deleted FROM t`, true}, // column names are not keywords
		{`INSERT INTO users(email) VALUES (?) RETURNING id`, false},
		{`UPDATE users SET name = ? WHERE id = ? RETURNING id`, false},
		{`DELETE FROM sessions WHERE id = ? RETURNING id`, false},
		{`WITH x AS (SELECT 1) INSERT INTO t SELECT * FROM x`, false},
		{`WITH x AS (SELECT 1) DELETE FROM t WHERE id IN x`, false},
		{`with x as (select 1) update t set a = 1`, false},
		{`PRAGMA wal_checkpoint`, false},
		{`pragma table_info(users)`, false},
		{`VACUUM INTO ?`, false},
		{`EXPLAIN SELECT 1`, false},
		{``, false},
		{`-- only a comment`, false},
		{`/* unterminated`, false},
		{`SELECTX 1`, false},
	}
	for _, c := range cases {
		if got := readOnlySQL(c.q); got != c.read {
			t.Errorf("readOnlySQL(%q) = %v, want %v", c.q, got, c.read)
		}
	}
}

func TestFileTarget(t *testing.T) {
	for target, want := range map[string]bool{
		"/data/corvus.db": true, `C:\x\corvus.db`: true,
		"": false, ":memory:": false, ":MEMORY:": false, "file::memory:?cache=shared": false, "x?mode=memory": false,
	} {
		if got := fileTarget(target); got != want {
			t.Errorf("fileTarget(%q) = %v, want %v", target, got, want)
		}
	}
}

func TestMemoryStoreHasNoReadPool(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.rdb != nil {
		t.Fatal("an in-memory database must not get a second pool (it would be a different database)")
	}
	ctx := context.Background()
	if _, err := s.CreateUser(ctx, "a@x", "A", "h", "admin"); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CountUsers(ctx); err != nil || n != 1 {
		t.Fatalf("count = %d, %v", n, err)
	}
}

// A long write transaction (a deploy) must not stall reads; writes wait for it.
func TestReadsDoNotWaitForTransaction(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.rdb == nil {
		t.Fatal("file database must have a read pool")
	}
	ctx := context.Background()
	a, err := s.CreateUser(ctx, "a@x", "A", "h", "admin")
	if err != nil {
		t.Fatal(err)
	}
	// read-your-writes outside a transaction
	if got, err := s.UserByEmail(ctx, "a@x"); err != nil || got.ID != a.ID {
		t.Fatalf("committed write not visible to reads: %v, %v", got, err)
	}
	if err := s.CreateSession(ctx, "tok", a.ID, time.Hour, "", ""); err != nil {
		t.Fatal(err)
	}
	// make the session due for its activity touch (a write)
	if _, err := s.db.Exec(`UPDATE sessions SET created_at = ?`, now().Add(-5*time.Minute)); err != nil {
		t.Fatal(err)
	}

	const hold = 2 * time.Second
	inTx := make(chan struct{})
	txDone := make(chan error, 1)
	go func() {
		txDone <- s.WithTx(ctx, func(tx *Store) error {
			if _, err := tx.CreateUser(ctx, "b@x", "B", "h", "admin"); err != nil {
				return err
			}
			if _, err := tx.q.ExecContext(ctx, `UPDATE users SET name = 'A2' WHERE id = ?`, a.ID); err != nil {
				return err
			}
			close(inTx)
			time.Sleep(hold)
			return nil
		})
	}()
	<-inTx
	start := time.Now()

	var wg sync.WaitGroup
	var writeErr error
	var writeTook time.Duration
	wg.Add(1)
	go func() { // a concurrent write waits for the transaction and then succeeds
		defer wg.Done()
		t0 := time.Now()
		writeErr = s.TouchLogin(ctx, a.ID)
		writeTook = time.Since(t0)
	}()

	timed := func(name string, limit time.Duration, fn func() error) {
		t.Helper()
		t0 := time.Now()
		if err := fn(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if d := time.Since(t0); d > limit {
			t.Fatalf("%s took %v during a transaction (limit %v)", name, d, limit)
		}
	}
	timed("CountUsers", 300*time.Millisecond, func() error {
		n, err := s.CountUsers(ctx)
		if err == nil && n != 1 {
			err = errors.New("uncommitted insert visible")
		}
		return err
	})
	timed("UserByID", 300*time.Millisecond, func() error {
		u, err := s.UserByID(ctx, a.ID)
		if err == nil && u.Name != "A" {
			err = errors.New("uncommitted update visible: " + u.Name)
		}
		return err
	})
	timed("ListProxyHosts", 300*time.Millisecond, func() error { _, err := s.ListProxyHosts(ctx); return err })
	// the session touch is skipped rather than waiting for the writer
	timed("SessionUser", sessionTouchWait+300*time.Millisecond, func() error {
		u, err := s.SessionUser(ctx, "tok")
		if err == nil && u.ID != a.ID {
			err = errors.New("wrong user")
		}
		return err
	})
	if time.Since(start) > hold-200*time.Millisecond {
		t.Fatal("reads did not finish while the transaction was open")
	}

	if err := <-txDone; err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if writeErr != nil {
		t.Fatalf("concurrent write failed: %v", writeErr)
	}
	if writeTook < hold/2 {
		t.Fatalf("concurrent write took %v; it should have waited for the transaction", writeTook)
	}
	if n, _ := s.CountUsers(ctx); n != 2 {
		t.Fatalf("after commit count = %d, want 2", n)
	}
	u, _ := s.UserByID(ctx, a.ID)
	if u.Name != "A2" || u.LastLoginAt == nil {
		t.Fatalf("after commit: %+v", u)
	}
	// the touch was skipped during the transaction and happens on the next request
	if _, err := s.SessionUser(ctx, "tok"); err != nil {
		t.Fatal(err)
	}
	var ls sql.NullTime
	if err := s.db.QueryRow(`SELECT last_seen_at FROM sessions WHERE token_hash = 'tok'`).Scan(&ls); err != nil {
		t.Fatal(err)
	}
	if !ls.Valid {
		t.Fatal("session activity not recorded once the writer was free")
	}
}

// A rejected transaction leaves reads on the committed state.
func TestReadsAfterRollback(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	boom := errors.New("rejected")
	if err := s.WithTx(ctx, func(tx *Store) error {
		if _, err := tx.CreateUser(ctx, "a@x", "A", "h", "admin"); err != nil {
			return err
		}
		return boom
	}); !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if n, _ := s.CountUsers(ctx); n != 0 {
		t.Fatalf("rolled-back insert visible: %d", n)
	}
}

func TestCloseClosesBothPools(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.rdb.Ping(); err == nil {
		t.Fatal("read pool still open")
	}
	if err := s.db.Ping(); err == nil {
		t.Fatal("writer still open")
	}
}
