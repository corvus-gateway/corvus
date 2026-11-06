package compiler

import (
	"context"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestInternalTarget(t *testing.T) {
	p := Paths{StatusPort: 9180, ACMEPort: 9181, RedisPort: 6379, AdminPort: 9443}
	for _, c := range []struct {
		addr string
		port int
		bad  bool
	}{
		{"127.0.0.1", 9180, true}, {"localhost", 9181, true}, {"[::1]", 6379, true}, {"127.0.0.2", 9180, true},
		{"127.0.0.1", 8080, false}, {"127.0.0.1", 9443, false}, {"172.17.0.1", 9180, false}, {"redis", 6379, false},
	} {
		if got := p.InternalTarget(c.addr, c.port) != ""; got != c.bad {
			t.Errorf("%s:%d refused=%v, want %v", c.addr, c.port, got, c.bad)
		}
	}
}

// With loopback staying loopback (host networking or CORVUS_LOOPBACK=container), a stream
// or proxy host may not publish Corvus's internal services.
func TestRenderRefusesInternalTargets(t *testing.T) {
	ctx := context.Background()
	c, st, _, dir := streamEnv(t)
	c.HostGateway = func() string { return "" }
	s := &store.Stream{IncomingPort: 8091, TCP: true, ForwardHost: "127.0.0.1", ForwardPort: 9180, Enabled: true}
	if err := st.SaveStream(ctx, s); err != nil {
		t.Fatal(err)
	}
	err := c.Render(ctx, st, dir+"/v1")
	if err == nil || !strings.Contains(err.Error(), "used internally") {
		t.Fatalf("stream to the status port rendered: %v", err)
	}
	// Mapped to the Docker host, the same target is a different machine: allowed.
	c.HostGateway = func() string { return "172.17.0.1" }
	if err := c.Render(ctx, st, dir+"/v2"); err != nil {
		t.Fatalf("stream to the Docker host refused: %v", err)
	}
}
