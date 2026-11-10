package backup

import (
	"archive/tar"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStageRejectsTooManyEntries(t *testing.T) {
	old := maxEntries
	maxEntries = 10
	t.Cleanup(func() { maxEntries = old })

	ok := []entry{{name: "secrets/data_key", body: "KEY"}, {name: "corvus.db", body: "DB"}}
	// empty files and directories cost nothing against the size limit
	many := append([]entry{}, ok...)
	for i := 0; i < 10; i++ {
		many = append(many, entry{name: fmt.Sprintf("certs/%d", i), typ: tar.TypeDir}, entry{name: fmt.Sprintf("certs/%d/empty", i)})
	}
	d := t.TempDir()
	_, err := Stage(bytes.NewReader(build(t, many, manifestFor(many))), d, 1<<20)
	if err == nil || !strings.Contains(err.Error(), "more than 10 entries") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(d, UploadDir)); !os.IsNotExist(err) {
		t.Fatal("partial upload left behind")
	}
	// within the limit the same kind of archive is accepted
	few := append(append([]entry{}, ok...), entry{name: "certs/1", typ: tar.TypeDir}, entry{name: "certs/1/empty"})
	if _, err := Stage(bytes.NewReader(build(t, few, manifestFor(few))), t.TempDir(), 1<<20); err != nil {
		t.Fatal(err)
	}
}
