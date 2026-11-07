package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// Two administrators demoting each other at the same moment: exactly one change wins
// and one active administrator remains.
func TestKeepingAdminConcurrent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	a, err := s.CreateUser(ctx, "a@example.com", "A", "x", "admin")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateUser(ctx, "b@example.com", "B", "x", "admin")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, u := range []*User{a, b} {
		wg.Add(1)
		go func(i int, u *User) {
			defer wg.Done()
			errs[i] = s.KeepingAdmin(ctx, func(tx *Store) error {
				if err := tx.UpdateUser(ctx, u.ID, u.Name, "operator", false); err != nil {
					return err
				}
				time.Sleep(50 * time.Millisecond) // widen the window between change and check
				return nil
			})
		}(i, u)
	}
	wg.Wait()
	lost := 0
	for _, err := range errs {
		switch {
		case errors.Is(err, ErrLastAdmin):
			lost++
		case err != nil:
			t.Fatal(err)
		}
	}
	if lost != 1 {
		t.Fatalf("want exactly one rejected change, got %d (%v)", lost, errs)
	}
	if n, err := s.CountActiveAdmins(ctx, 0); err != nil || n != 1 {
		t.Fatalf("active admins = %d, %v; want 1", n, err)
	}

	// deleting the only remaining admin is rolled back
	left := a
	if u, _ := s.UserByID(ctx, a.ID); u.Role != "admin" {
		left = b
	}
	err = s.KeepingAdmin(ctx, func(tx *Store) error { return tx.DeleteUser(ctx, left.ID) })
	if !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete last admin: %v", err)
	}
	if _, err := s.UserByID(ctx, left.ID); err != nil {
		t.Fatalf("last admin gone after rollback: %v", err)
	}
}
