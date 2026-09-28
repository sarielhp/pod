package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreserveOriginalBeforeCutRefusesADanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "ep.mp3")
	if err := os.WriteFile(main, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "victim")
	precut := main + ".precut"
	if err := os.Symlink(target, precut); err != nil {
		t.Fatal(err)
	}
	if err := preserveOriginalBeforeCut(main, precut); err == nil {
		t.Fatal("a dangling .precut symlink was accepted as the backup location")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("the backup was written through the symlink to its target")
	}
}

func TestPreserveOriginalBeforeCutFailsWhenNoBackupCanBeMade(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	dir := t.TempDir()
	main := filepath.Join(dir, "ep.mp3")
	if err := os.WriteFile(main, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })
	if err := preserveOriginalBeforeCut(main, main+".precut"); err == nil {
		t.Fatal("no backup could be created, yet no error was returned; the cut would overwrite the only copy")
	}
}

func TestPreserveOriginalBeforeCutKeepsACopy(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "ep.mp3")
	if err := os.WriteFile(main, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	precut := main + ".precut"
	if err := preserveOriginalBeforeCut(main, precut); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(precut)
	if err != nil || string(data) != "audio" {
		t.Fatalf("precut content %q err %v", data, err)
	}
	if err := preserveOriginalBeforeCut(main, precut); err != nil {
		t.Fatalf("second call with an existing backup: %v", err)
	}
}
