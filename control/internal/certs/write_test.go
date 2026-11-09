package certs

import (
	"os"
	"path/filepath"
	"testing"
)

// Write replaces both files and leaves no temporary files behind.
func TestWriteReplacesPair(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c1")
	if err := Write(dir, []byte("cert-1"), []byte("key-1")); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, []byte("cert-2"), []byte("key-2")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"fullchain.pem": "cert-2", "privkey.pem": "key-2"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(b) != want {
			t.Errorf("%s = %q, %v; want %q", name, b, err, want)
		}
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 2 {
		t.Errorf("want 2 files, got %d", len(ents))
	}
}
