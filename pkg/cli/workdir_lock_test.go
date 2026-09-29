package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/util"
)

func podcastDirWithWork(t *testing.T) (dir, mp3, work string) {
	t.Helper()
	dir = t.TempDir()
	mp3 = filepath.Join(dir, "ep.mp3")
	work = filepath.Join(dir, ".work")
	if err := os.WriteFile(mp3, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(work, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "ep.wav"), []byte("scratch"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir, mp3, work
}

func TestRemoveWorkDirsLeavesALiveWorkersScratchAlone(t *testing.T) {
	dir, mp3, work := podcastDirWithWork(t)
	lock, err := util.AcquireFileLock(mp3)
	if err != nil || lock == nil {
		t.Fatalf("test lock: %v", err)
	}
	removeWorkDirs(dir)
	if _, err := os.Stat(work); err != nil {
		t.Fatal(".work of an episode locked by another instance was deleted")
	}
	lock.Release()
	removeWorkDirs(dir)
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Fatal("idle .work survived the sweep")
	}
}

func TestCutAndTranscribeRefuseAnEpisodeAnotherInstanceHolds(t *testing.T) {
	_, mp3, _ := podcastDirWithWork(t)
	lock, err := util.AcquireFileLock(mp3)
	if err != nil || lock == nil {
		t.Fatalf("test lock: %v", err)
	}
	defer lock.Release()

	err = runCutCommand(Config{}, CLIOptions{Args: []string{mp3}, ProcOptions: ProcOptions{Quiet: true}})
	if err == nil || !strings.Contains(err.Error(), "another pod instance") {
		t.Fatalf("cut proceeded on a locked episode: %v", err)
	}
	_, err = transcribeOneLocked(transcribeTarget{source: mp3, lockPath: mp3}, Config{}, CLIOptions{ProcOptions: ProcOptions{Quiet: true}}, ProcOptions{Quiet: true})
	if err == nil || !strings.Contains(err.Error(), "another pod instance") {
		t.Fatalf("transcribe proceeded on a locked episode: %v", err)
	}
}
