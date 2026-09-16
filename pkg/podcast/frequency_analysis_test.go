package podcast

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
)

// pathSafeTitle must keep spaces and punctuation that a filesystem accepts.
// SanitizeTitle, which strips a title to letters and digits, would turn
// "Ep. 5: The Long Now" into a different directory name entirely.
func TestPathSafeTitleReplacesOnlyIllegalCharacters(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Ep. 5: The Long Now": "Ep. 5_ The Long Now",
		"A/B Testing":         "A_B Testing",
		`Quote"Me`:            "Quote_Me",
		"Pipe|Star*Question?": "Pipe_Star_Question_",
		"Plain Title":         "Plain Title",
		"Angle<>Brackets":     "Angle__Brackets",
		`Back\Slash`:          "Back_Slash",
	}
	for in, want := range cases {
		if got := pathSafeTitle(in); got != want {
			t.Errorf("pathSafeTitle(%q) = %q, want %q", in, got, want)
		}
	}
	if got := pathSafeTitle("Ep. 5: The Long Now"); got == SanitizeTitle("Ep. 5: The Long Now") {
		t.Error("pathSafeTitle should not agree with SanitizeTitle; they are for different jobs")
	}
}

func TestAnalyzeFrequenciesRecordsCadenceInPodcastConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "Show")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ep.mp3"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	item := backend.Podcast{ID: "p1", RelPath: "Show", Path: dir}
	item.Media.Metadata.Title = "Show"
	// Weekly releases, so the analysis has something to classify.
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 8 {
		item.Media.Episodes = append(item.Media.Episodes, backend.Episode{
			Title:       "Ep",
			PublishedAt: base.AddDate(0, 0, -7*i).UnixMilli(),
		})
	}

	lib := Open(Config{PodcastsDir: root}, nil, nil)
	results := lib.AnalyzeFrequencies([]backend.Podcast{item}, FrequencyOptions{})
	if results[0].Err != nil {
		t.Fatalf("analysis failed: %v", results[0].Err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].Title != "Show" {
		t.Errorf("title = %q", results[0].Title)
	}
	if results[0].PodDir != dir {
		t.Errorf("PodDir = %q, want %q", results[0].PodDir, dir)
	}
	if !results[0].PolicySaved {
		t.Error("the analysis should have been written to the podcast config")
	}
	if cfg := config.LoadPodcastConfig(dir, config.PodcastConfig{}); cfg.Frequency == nil {
		t.Error("Frequency not recorded in podcast.json")
	}
}

// Without DisableHourly, a podcast with no directory gets none created: the
// run is read-only apart from recording cadence where there is already a home
// for it.
func TestAnalyzeFrequenciesDoesNotCreateDirectoriesByDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	item := backend.Podcast{ID: "p1"}
	item.Media.Metadata.Title = "Nowhere Show"

	lib := Open(Config{PodcastsDir: root}, nil, nil)
	results := lib.AnalyzeFrequencies([]backend.Podcast{item}, FrequencyOptions{})

	if results[0].PodDir != "" {
		t.Errorf("PodDir = %q, want empty", results[0].PodDir)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the library root gained %d entries", len(entries))
	}
}

func TestSelectBackendTargets(t *testing.T) {
	t.Parallel()
	withFeed := backend.Podcast{ID: "a"}
	withFeed.Media.Metadata.Title = "Alpha"
	withFeed.Media.Metadata.FeedURL = "https://example.com/a.xml"

	noFeed := backend.Podcast{ID: "b"}
	noFeed.Media.Metadata.Title = "Beta"

	all := []backend.Podcast{withFeed, noFeed}
	lib := Open(Config{PodcastsDir: t.TempDir()}, nil, nil)

	got, err := lib.SelectBackendTargets(all, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Errorf("an empty query should keep only podcasts with a feed: %+v", got)
	}

	// Named explicitly, a podcast without a feed is still selected.
	got, err = lib.SelectBackendTargets(all, "Beta")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("naming a podcast should select it regardless of feed: %+v", got)
	}
}

// When nothing has a feed, returning the whole list beats returning nothing.
func TestWithFeedURLFallsBackToEverything(t *testing.T) {
	t.Parallel()
	a := backend.Podcast{ID: "a"}
	b := backend.Podcast{ID: "b"}
	if got := withFeedURL([]backend.Podcast{a, b}); len(got) != 2 {
		t.Errorf("got %d, want both podcasts back", len(got))
	}
}
