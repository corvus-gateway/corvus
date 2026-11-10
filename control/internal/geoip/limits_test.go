package geoip

import (
	"net"
	"strings"
	"testing"
)

func TestParseCSVRowLimit(t *testing.T) {
	old := maxRows
	maxRows = 3
	t.Cleanup(func() { maxRows = old })
	ok := "1.0.0.0,1.0.0.255,AU\n2.0.0.0,2.0.0.255,FR\n2001:200::,2001:200::ffff,JP\n# comment\n"
	if _, err := ParseCSV(strings.NewReader(ok)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCSV(strings.NewReader(ok + "3.0.0.0,3.0.0.255,US\n")); err == nil || !strings.Contains(err.Error(), "more than 3 ranges") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseCSVRejectsMalformedRanges(t *testing.T) {
	for name, in := range map[string]string{
		"inverted v4":  "1.0.0.255,1.0.0.0,AU\n",
		"inverted v6":  "2001:200::ffff,2001:200::,JP\n",
		"mixed family": "1.0.0.0,2001:200::ffff,AU\n",
	} {
		if _, err := ParseCSV(strings.NewReader(in)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// overlapping and unsorted input still yields a sorted table
	db, err := ParseCSV(strings.NewReader("1.0.1.0,1.0.1.255,CN\n1.0.0.0,1.0.1.127,AU\n"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(db.v4); i++ {
		if db.v4[i].start <= db.v4[i-1].start {
			t.Fatalf("unsorted table: %v", db.v4)
		}
	}
	if db.Lookup(net.ParseIP("1.0.1.100")) != "AU" || db.Lookup(net.ParseIP("1.0.1.200")) != "CN" {
		t.Fatal("lookup")
	}
}
