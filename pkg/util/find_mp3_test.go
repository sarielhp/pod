package util

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestFindMP3FilesListsAnAliasedPodcastOnceUnderItsRealPath(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "library")
	show := filepath.Join(real, "Dan_Snows_History_Hit")
	if err := os.MkdirAll(show, 0755); err != nil {
		t.Fatal(err)
	}
	ep := filepath.Join(show, "ep.mp3")
	if err := os.WriteFile(ep, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"Dan Snow's History Hit", "Dan Snows History Hit"} {
		if err := os.Symlink("Dan_Snows_History_Hit", filepath.Join(real, alias)); err != nil {
			t.Fatal(err)
		}
	}
	root := filepath.Join(base, "media")
	if err := os.Symlink(real, root); err != nil {
		t.Fatal(err)
	}

	got := FindMP3Files(root)
	sort.Strings(got)
	want := []string{filepath.Join(root, "Dan_Snows_History_Hit", "ep.mp3")}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("a show reachable through a symlinked root and two alias links was listed as %v, want exactly %v", got, want)
	}
}

func TestFindMP3FilesStillFollowsASymlinkToAnOutsideDirectory(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "library")
	outside := filepath.Join(base, "elsewhere")
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "far.mp3"), []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "Linked_Show")); err != nil {
		t.Fatal(err)
	}
	got := FindMP3Files(root)
	want := filepath.Join(root, "Linked_Show", "far.mp3")
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %v, want [%s]", got, want)
	}
}
