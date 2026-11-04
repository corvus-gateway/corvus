package validate

import (
	"testing"

	"corvus/control/internal/store"
)

func TestStream(t *testing.T) {
	cert := int64(1)
	st := store.Stream{IncomingPort: 5432, TCP: true, ForwardHost: " DB.internal ", ForwardPort: 5432, CertificateID: &cert}
	if err := Stream(&st); err != nil {
		t.Fatal(err)
	}
	if st.ForwardHost != "db.internal" {
		t.Fatalf("not normalized: %+v", st)
	}
	for _, ok := range []string{"10.0.0.1", "demo", "host.docker.internal", "a_b.example.com"} {
		st := store.Stream{IncomingPort: 53, UDP: true, ForwardHost: ok, ForwardPort: 53}
		if err := Stream(&st); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	cases := map[string]func(st *store.Stream){
		"port 0":            func(st *store.Stream) { st.IncomingPort = 0 },
		"port too large":    func(st *store.Stream) { st.IncomingPort = 65536 },
		"forward port 0":    func(st *store.Stream) { st.ForwardPort = 0 },
		"no protocol":       func(st *store.Stream) { st.TCP, st.UDP = false, false },
		"host injection":    func(st *store.Stream) { st.ForwardHost = "db; include /etc/passwd" },
		"host with port":    func(st *store.Stream) { st.ForwardHost = "db:5432" },
		"host empty":        func(st *store.Stream) { st.ForwardHost = "" },
		"host ipv6":         func(st *store.Stream) { st.ForwardHost = "::1" },
		"host variable":     func(st *store.Stream) { st.ForwardHost = "$host" },
		"tls without tcp":   func(st *store.Stream) { st.TCP, st.UDP, st.CertificateID = false, true, &cert },
		"host with newline": func(st *store.Stream) { st.ForwardHost = "db\nlisten 1" },
	}
	for name, mutate := range cases {
		st := store.Stream{IncomingPort: 5432, TCP: true, ForwardHost: "db", ForwardPort: 5432}
		mutate(&st)
		if err := Stream(&st); err == nil {
			t.Errorf("%s: accepted %+v", name, st)
		}
	}
}
