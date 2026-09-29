package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/config"
	"pod/pkg/podcast"
)

func libraryEpisode(t *testing.T, precut bool) (Config, string, string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "Show")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.SavePodcastConfig(dir, config.PodcastConfig{}); err != nil {
		t.Fatal(err)
	}
	audio := filepath.Join(dir, "2026-01-01_abc.mp3")
	files := []string{audio}
	if precut {
		files = append(files, audio+".precut")
	}
	for _, f := range files {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	id := podcast.GetOrSetEpisodeShortID(dir, "", audio)
	return Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}, audio, id
}

func TestAnEpisodeIDIsTranscribedFromItsUncutOriginalAndNamedAfterTheEpisode(t *testing.T) {
	cfg, audio, id := libraryEpisode(t, true)
	targets, err := resolveTranscribeArgs(cfg, []string{id})
	if err != nil || len(targets) != 1 {
		t.Fatalf("targets %v, err %v", targets, err)
	}
	got := targets[0]
	if got.source != audio+".precut" || got.outputBase != strings.TrimSuffix(audio, ".mp3") || got.lockPath != audio || !got.isEpisode() {
		t.Fatalf("target %+v", got)
	}
}

func TestAnEpisodeWithoutAnOriginalIsTranscribedFromItsAudio(t *testing.T) {
	cfg, audio, id := libraryEpisode(t, false)
	targets, err := resolveTranscribeArgs(cfg, []string{id})
	if err != nil || targets[0].source != audio {
		t.Fatalf("targets %v, err %v", targets, err)
	}
}

func TestAPathOnDiskIsAFileNotAnEpisodeLookup(t *testing.T) {
	cfg, audio, _ := libraryEpisode(t, false)
	targets, err := resolveTranscribeArgs(cfg, []string{audio})
	if err != nil || len(targets) != 1 || targets[0].isEpisode() || targets[0].source != audio {
		t.Fatalf("targets %+v, err %v", targets, err)
	}
}

func TestAnUnknownArgumentSaysItIsNotAFile(t *testing.T) {
	cfg, _, _ := libraryEpisode(t, false)
	if _, err := resolveTranscribeArgs(cfg, []string{"zzzzzz"}); err == nil || !strings.Contains(err.Error(), "zzzzzz is not a file") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlainTranscriptionRefusesToReplaceAnEpisodesTranscriptButSpeakersDoNot(t *testing.T) {
	cfg, audio, id := libraryEpisode(t, false)
	if err := os.WriteFile(strings.TrimSuffix(audio, ".mp3")+".transcript.json", []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	targets, _ := resolveTranscribeArgs(cfg, []string{id})
	if err := refuseToReplaceTranscript(targets[0], false); err == nil || !strings.Contains(err.Error(), "--speakers") {
		t.Fatalf("err = %v", err)
	}
	if err := refuseToReplaceTranscript(targets[0], true); err != nil {
		t.Fatalf("a speaker version goes to its own file: %v", err)
	}
}
