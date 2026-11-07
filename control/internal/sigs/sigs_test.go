package sigs

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"

	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

func TestDefaultBundleTemplatesValidate(t *testing.T) {
	b, err := parse(defaultBundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Signatures) < 8 || len(b.Templates) < 10 {
		t.Fatalf("bundle too small: %d sigs, %d templates", len(b.Signatures), len(b.Templates))
	}
	for _, tpl := range b.Templates {
		for _, exempt := range []int64{0, 7} {
			r := tpl.Rule("log", 403, "all", nil, exempt)
			if err := validate.SecurityRule(&r, func(id int64) string {
				if id == 7 {
					return store.ListKindIP
				}
				return ""
			}, nil); err != nil {
				t.Errorf("template %s (exempt %d): %v", tpl.ID, exempt, err)
			}
		}
	}
}

func TestInstallSignedNewerOnly(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	old := PublicKey
	PublicKey = base64.StdEncoding.EncodeToString(pub)
	defer func() { PublicKey = old }()

	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Service{Dir: filepath.Join(dir, "rules"), Store: st}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	b, _ := parse(defaultBundle)
	b.Serial++
	b.Version = "test-2"
	data, _ := json.Marshal(b)
	sign := func(d []byte) []byte { return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, d))) }
	ctx := context.Background()

	if _, err := s.Install(ctx, data, []byte("bm90LWEtc2ln")); err == nil {
		t.Fatal("bad signature accepted")
	}
	tampered := append([]byte{}, data...)
	tampered[len(tampered)-2] = ' '
	if _, err := s.Install(ctx, tampered, sign(data)); err == nil {
		t.Fatal("tampered bundle accepted")
	}
	st1, err := s.Install(ctx, data, sign(data))
	if err != nil || st1.Version != "test-2" || st1.Source != "installed" {
		t.Fatalf("install: %+v %v", st1, err)
	}
	if _, err := s.Install(ctx, data, sign(data)); err != ErrNotNewer {
		t.Fatalf("re-install/downgrade: %v", err)
	}
	// survives a restart
	s2 := &Service{Dir: filepath.Join(dir, "rules"), Store: st}
	_ = s2.Init()
	if s2.Bundle().Version != "test-2" {
		t.Fatal("installed bundle not loaded at start")
	}
}
