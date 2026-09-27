package episode

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/types"
)

func TestIsEpisodeCleanRequiresNonEmptyTranscript(t *testing.T) {
	t.Parallel()
	audioPath := filepath.Join(t.TempDir(), "episode.mp3")
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	st := GetOrCreateEpisodeStatus(audioPath)
	st.Status = types.StateDone
	if err := SaveEpisodeStatus(StatusPathFor(audioPath), st); err != nil {
		t.Fatal(err)
	}
	if IsEpisodeClean(audioPath) {
		t.Fatal("completed episode without a transcript is clean")
	}

	transcriptPath := filepath.Join(filepath.Dir(audioPath), "episode.transcript.json")
	if err := os.WriteFile(transcriptPath, []byte(`{"text":"This episode contains a complete discussion with enough meaningful transcript text."}`), 0644); err != nil {
		t.Fatal(err)
	}
	if !IsEpisodeClean(audioPath) {
		t.Fatal("completed episode with a non-empty transcript is not clean")
	}
}

func TestGetOrCreateEpisodeStatusFavoriteInheritance(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "FavPodcast")
	_ = os.MkdirAll(podDir, 0755)

	cutoff := time.Now().Add(-5 * time.Minute).UTC()
	podCfgJSON := fmt.Sprintf(`{"favorite":true,"favorite_since":%q}`, cutoff.Format(time.RFC3339))
	_ = os.WriteFile(filepath.Join(podDir, "podcast.json"), []byte(podCfgJSON), 0644)

	// Pre-existing episode before cutoff
	oldAudioPath := filepath.Join(podDir, "old_episode.mp3")
	_ = os.WriteFile(oldAudioPath, []byte("audio"), 0644)
	oldTime := cutoff.Add(-10 * time.Minute)
	_ = os.Chtimes(oldAudioPath, oldTime, oldTime)

	oldSt := GetOrCreateEpisodeStatus(oldAudioPath)
	if oldSt.IsFavorite() {
		t.Errorf("expected pre-existing episode status to NOT inherit favorite")
	}

	// Newly downloaded episode after cutoff
	newAudioPath := filepath.Join(podDir, "new_episode.mp3")
	_ = os.WriteFile(newAudioPath, []byte("audio"), 0644)
	newTime := cutoff.Add(10 * time.Minute)
	_ = os.Chtimes(newAudioPath, newTime, newTime)

	newSt := GetOrCreateEpisodeStatus(newAudioPath)
	if !newSt.IsFavorite() {
		t.Errorf("expected newly downloaded episode status to inherit favorite")
	}

	loaded, err := LoadEpisodeStatus(StatusPathFor(newAudioPath))
	if err != nil {
		t.Fatalf("LoadEpisodeStatus failed: %v", err)
	}
	if !loaded.IsFavorite() || !loaded.Favorite {
		t.Errorf("expected loaded episode status to have favorite: true")
	}
}
