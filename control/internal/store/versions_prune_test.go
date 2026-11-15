package store

import (
	"context"
	"testing"
)

func TestPruneVersions(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	add := func(status string, rollbackOf *int64) int64 {
		t.Helper()
		v := &ConfigVersion{Status: status, RollbackOf: rollbackOf, Dir: "x"}
		if err := s.CreateVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
		return v.ID
	}
	target := add("superseded", nil) // 1: rollback target
	for i := 0; i < 4; i++ {
		add("superseded", nil) // 2, 4, 6, 8
		add("failed", nil)     // 3, 5, 7, 9
	}
	rb := add("superseded", &target)
	add("superseded", nil)
	active := add("active", nil)

	n, err := s.PruneVersions(ctx, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	vs, _ := s.ListVersions(ctx, 100, 0)
	kept := map[int64]bool{}
	failed := 0
	for _, v := range vs {
		kept[v.ID] = true
		if v.Status == "failed" {
			failed++
		}
	}
	// deployed: active + the 2 before it (rb among them) + the rollback target
	for _, id := range []int64{active, rb, target} {
		if !kept[id] {
			t.Errorf("version %d pruned", id)
		}
	}
	if failed != 2 || len(vs) != 6 || n != 12-6 {
		t.Fatalf("kept %d rows (%d failed), pruned %d", len(vs), failed, n)
	}

	// pruning again changes nothing; a gap in ids is never refilled
	if n, _ := s.PruneVersions(ctx, 3, 2); n != 0 {
		t.Fatalf("second prune removed %d", n)
	}
	if err := s.DeleteVersion(ctx, active); err != nil {
		t.Fatal(err)
	}
	if id := add("active", nil); id <= active {
		t.Fatalf("id %d reused (deleted %d)", id, active)
	}
}
