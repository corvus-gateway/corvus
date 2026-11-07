package validate

import (
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestNotes(t *testing.T) {
	st := &store.Stream{IncomingPort: 5432, TCP: true, ForwardHost: "db", ForwardPort: 5432, Notes: "  Postgres for the shop\r\nask the shop team before changing  "}
	if err := Stream(st); err != nil {
		t.Fatal(err)
	}
	if st.Notes != "Postgres for the shop\nask the shop team before changing" {
		t.Fatalf("notes = %q", st.Notes)
	}
	for _, bad := range []string{strings.Repeat("가", MaxNotes+1), "a\x00b", "bell\a"} {
		st.Notes = bad
		if err := Stream(st); err == nil || !strings.Contains(err.Error(), "notes") {
			t.Fatalf("notes %q: err = %v", bad, err)
		}
	}
	st.Notes = strings.Repeat("가", MaxNotes)
	if err := Stream(st); err != nil {
		t.Fatalf("max length: %v", err)
	}
}
