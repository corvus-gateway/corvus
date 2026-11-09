package live

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"corvus/control/internal/store"
)

// fakeRedis is a minimal RESP2 server: enough of PING, EVALSHA/EVAL and DEL for the
// client in this package. It records every command it receives.
type fakeRedis struct {
	mu    sync.Mutex
	cmds  [][]string
	stats string // reply to EVAL (RESP)
	l     net.Listener
}

func startFakeRedis(t *testing.T) *fakeRedis {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRedis{l: l, stats: "*5\r\n:1\r\n:0\r\n:0\r\n:7\r\n:3\r\n"}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("unexpected %q", line)
	}
	n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, _ := strconv.Atoi(strings.TrimSpace(hdr[1:]))
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		out = append(out, string(buf[:size]))
	}
	return out, nil
}

func (f *fakeRedis) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	for {
		cmd, err := readCommand(r)
		if err != nil {
			return
		}
		f.mu.Lock()
		f.cmds = append(f.cmds, cmd)
		stats := f.stats
		f.mu.Unlock()
		var reply string
		switch strings.ToUpper(cmd[0]) {
		case "PING":
			reply = "+PONG\r\n"
		case "EVALSHA":
			reply = "-NOSCRIPT No matching script\r\n"
		case "EVAL":
			reply = stats
		case "DEL":
			reply = ":" + strconv.Itoa(len(cmd)-1) + "\r\n"
		case "HELLO":
			reply = "-ERR unknown command 'HELLO'\r\n"
		default:
			reply = "+OK\r\n"
		}
		if _, err := c.Write([]byte(reply)); err != nil {
			return
		}
	}
}

func (f *fakeRedis) find(name string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.cmds {
		if strings.EqualFold(c[0], name) {
			return c
		}
	}
	return nil
}

func writeScript(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "waitroom.lua")
	if err := os.WriteFile(p, []byte("return {1,0,0,0,0}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCovNewMissingScript(t *testing.T) {
	if _, err := New("127.0.0.1:1", "", "", filepath.Join(t.TempDir(), "nope.lua")); err == nil || !strings.Contains(err.Error(), "access queue script") {
		t.Fatalf("err = %v", err)
	}
}

func TestCovNilLive(t *testing.T) {
	var l *Live
	ctx := context.Background()
	st := l.Room(ctx, store.Waitroom{Name: "q", Capacity: 5})
	if st.Available || st.Name != "q" || st.Capacity != 5 {
		t.Fatalf("nil live: %+v", st)
	}
	if l.Ping(ctx) {
		t.Fatal("nil live is not reachable")
	}
	if err := l.Forget(ctx, "q"); err != nil {
		t.Fatal(err)
	}
}

func TestCovLiveAgainstFakeRedis(t *testing.T) {
	f := startFakeRedis(t)
	l, err := New(f.l.Addr().String(), "", "", writeScript(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if !l.Ping(ctx) {
		t.Fatal("ping")
	}
	w := store.Waitroom{Name: "sale", Capacity: 100, ActiveTTL: 60, GrantTTL: 30, QueueTTL: 600, AdmitRate: 5}
	st := l.Room(ctx, w)
	if !st.Available || st.Waiting != 7 || st.Active != 3 || st.Capacity != 100 || st.Name != "sale" {
		t.Fatalf("room: %+v", st)
	}
	ev := f.find("EVAL")
	if ev == nil {
		t.Fatal("no EVAL sent")
	}
	// EVAL script numkeys k1..k6 op sid capacity activeTTLms grantTTLms queueTTLms rate 200
	if ev[2] != "6" || ev[3] != "corvus:wr:{sale}:active" || ev[8] != "corvus:wr:{sale}:granted" || ev[9] != "stats" ||
		ev[11] != "100" || ev[12] != "60000" || ev[13] != "30000" || ev[14] != "600000" || ev[15] != "5" {
		t.Fatalf("EVAL args: %q", ev)
	}

	if err := l.Forget(ctx, "sale"); err != nil {
		t.Fatal(err)
	}
	del := f.find("DEL")
	if len(del) != 7 || del[1] != "corvus:wr:{sale}:active" || del[6] != "corvus:wr:{sale}:granted" {
		t.Fatalf("DEL: %q", del)
	}

	// a short reply means the queue state is not available
	f.mu.Lock()
	f.stats = "*2\r\n:1\r\n:2\r\n"
	f.mu.Unlock()
	if st := l.Room(ctx, w); st.Available {
		t.Fatalf("short reply: %+v", st)
	}
}

func TestCovLiveUnreachable(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	l, err := New(addr, "u", "p", writeScript(t))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if l.Ping(ctx) {
		t.Fatal("closed port must not ping")
	}
	if st := l.Room(ctx, store.Waitroom{Name: "x"}); st.Available {
		t.Fatal("unavailable")
	}
	if err := l.Forget(ctx, "x"); err == nil {
		t.Fatal("forget on unreachable redis must fail")
	}
}

func TestCovACLFile(t *testing.T) {
	acl := ACLFile("gateway", "s3cret")
	sum := sha256.Sum256([]byte("s3cret"))
	lines := strings.Split(strings.TrimSpace(acl), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines: %q", lines)
	}
	if lines[0] != "user default off resetkeys resetchannels -@all" {
		t.Fatalf("default user: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "user gateway on #"+hex.EncodeToString(sum[:])+" resetkeys ~corvus:wr:* ") ||
		!strings.Contains(lines[1], "+evalsha") || !strings.Contains(lines[1], "+client|setinfo") || strings.Contains(acl, "s3cret") {
		t.Fatalf("gateway user: %q", lines[1])
	}
	if lines[2] != "user health on nopass resetkeys resetchannels -@all +ping" {
		t.Fatalf("health user: %q", lines[2])
	}

	p := filepath.Join(t.TempDir(), "redis", "users.acl")
	if err := WriteACL(p, "gateway", "s3cret"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != acl {
		t.Fatalf("written: %v", err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o644 {
		t.Fatalf("mode %v", fi.Mode())
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp file left")
	}
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, nil, 0o600)
	if err := WriteACL(filepath.Join(f, "sub", "users.acl"), "u", "p"); err == nil {
		t.Fatal("unusable dir must fail")
	}
	d := t.TempDir()
	_ = os.MkdirAll(filepath.Join(d, "users.acl.tmp", "x"), 0o755)
	if err := WriteACL(filepath.Join(d, "users.acl"), "u", "p"); err == nil {
		t.Fatal("unwritable tmp must fail")
	}
}
