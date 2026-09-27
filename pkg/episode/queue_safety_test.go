package episode

import (
	"os"
	"path/filepath"
	"pod/pkg/types"
	"testing"
)

func TestQueuePathsAndRemoval(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"a/podcast.mp3", "b/podcast.mp3", "precut.mp3", ".work/temp.mp3"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"../outside.mp3", filepath.Join(dir, "a/podcast.mp3"), "podcast.mp3", "precut.mp3", ".work/temp.mp3", "missing.mp3"} {
		if _, err := ResolveQueueAudioPath(dir, name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if err := os.Symlink(filepath.Join(dir, "a"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveQueueAudioPath(dir, "link/podcast.mp3"); err == nil {
		t.Fatal("accepted symlink")
	}
	for _, name := range []string{"a/podcast.mp3", "b/podcast.mp3", "podcast.mp3"} {
		if _, err := AddToQueueChecked(dir, name); err != nil {
			t.Fatal(err)
		}
	}
	if removed, err := RemoveQueuedAudio(dir, filepath.Join(dir, "a/podcast.mp3")); err != nil || !removed {
		t.Fatalf("remove: %v %v", removed, err)
	}
	entries, err := ReadQueue(dir)
	if err != nil || len(entries) != 2 || entries[0] != "b/podcast.mp3" || entries[1] != "podcast.mp3" {
		t.Fatalf("queue: %v %v", entries, err)
	}
}

func TestQueueErrorsPreserveFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.json")
	if err := os.WriteFile(path, []byte("broken"), 0644); err != nil {
		t.Fatal(err)
	}
	if added, err := AddToQueueChecked(dir, "episode.mp3"); err == nil || added {
		t.Fatal("corrupt queue accepted")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "broken" {
		t.Fatal("corrupt queue overwritten")
	}
	other := t.TempDir()
	if err := os.Mkdir(filepath.Join(other, "queue.json"), 0755); err != nil {
		t.Fatal(err)
	}
	if added, err := AddToQueueChecked(other, "episode.mp3"); err == nil || added {
		t.Fatal("read error hidden")
	}
}

func TestCleanRejectsInvalidTranscriptAndFailedDetection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "episode.mp3")
	if err := SaveEpisodeStatus(StatusPathFor(path), &types.EpisodeStatusFile{Status: types.StateDone}); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"", "{}", "broken", `{"text":"   "}`, `{"text":"too short"}`, `{"text":"This is a sufficiently long transcript for a real episode discussion.","ad_detection_successful":false}`} {
		if err := os.WriteFile(filepath.Join(dir, "episode.transcript.json"), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		if IsEpisodeClean(path) {
			t.Errorf("accepted transcript %q", data)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "episode.transcript.json"), []byte(`{"text":"This is a sufficiently long transcript for a real episode discussion."}`), 0644); err != nil {
		t.Fatal(err)
	}
	failed := false
	if err := SaveEpisodeStatus(StatusPathFor(path), &types.EpisodeStatusFile{Status: types.StateDone, AdDetectionSuccessful: &failed}); err != nil {
		t.Fatal(err)
	}
	if IsEpisodeClean(path) {
		t.Fatal("failed detection marked clean")
	}
}

func TestResolveQueueAudioPathNested(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	nestedDir := filepath.Join(dir, "Ballot Bots — The Weekly")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}
	audioPath := filepath.Join(nestedDir, "podcast.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, entry := range []string{"Ballot Bots — The Weekly.mp3", "Ballot Bots — The Weekly", "Ballot Bots — The Weekly/podcast.mp3"} {
		resolved, err := ResolveQueueAudioPath(dir, entry)
		if err != nil {
			t.Fatalf("failed to resolve %q: %v", entry, err)
		}
		if resolved != audioPath {
			t.Fatalf("expected %q, got %q", audioPath, resolved)
		}
	}
}
