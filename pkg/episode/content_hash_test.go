package episode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContentHashIsStableAndContentSensitive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := filepath.Join(dir, "a.mp3")
	b := filepath.Join(dir, "b.mp3")
	if err := os.WriteFile(a, []byte("episode audio with ads"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("episode audio with ads"), 0644); err != nil {
		t.Fatal(err)
	}

	ha, err := ContentHash(a)
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	hb, err := ContentHash(b)
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}
	if ha != hb {
		t.Errorf("identical bytes hashed differently: %s vs %s", ha, hb)
	}
	if len(ha) != 16 {
		t.Errorf("hash length = %d, want 16", len(ha))
	}

	if err := os.WriteFile(b, []byte("episode audio, ad-free"), 0644); err != nil {
		t.Fatal(err)
	}
	hb2, err := ContentHash(b)
	if err != nil {
		t.Fatalf("hash b2: %v", err)
	}
	if ha == hb2 {
		t.Errorf("changed bytes kept the same hash: %s", ha)
	}
}

func TestContentHashMissingFile(t *testing.T) {
	t.Parallel()

	if _, err := ContentHash(filepath.Join(t.TempDir(), "absent.mp3")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
