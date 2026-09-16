package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pod/pkg/config"
	"pod/pkg/types"
)

func TestCatalogKeyMatchesAFeedTitleToAFilename(t *testing.T) {
	t.Parallel()
	// A downloaded file's name was derived from the feed title but sanitised:
	// punctuation dropped, spaces turned to underscores, a date prefixed. Both
	// sides reduce to letters and digits so the two still meet.
	feed := catalogKey("p1", "48 Days Until the Midterms: Is a Blue Wave Coming?")
	file := catalogKey("p1", "48_Days_Until_the_Midterms_Is_a_Blue_Wave_Coming")
	if feed != file {
		t.Errorf("did not match:\n  feed=%q\n  file=%q", feed, file)
	}
}

func TestCatalogKeySeparatesPodcasts(t *testing.T) {
	t.Parallel()
	// Shows reuse episode titles constantly ("Episode 1", "Introduction").
	if catalogKey("p1", "Introduction") == catalogKey("p2", "Introduction") {
		t.Error("same title in different podcasts collapsed")
	}
}

func TestCatalogKeyIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	if catalogKey("p1", "The Headlines") != catalogKey("p1", "the headlines") {
		t.Error("case changed the key")
	}
}

func TestCollectCatalogEpisodesKeepsDownloadsMissingFromTheHistory(t *testing.T) {
	t.Parallel()
	// The retained history is capped, so an older download falls off it. It
	// must still be listed: it is an episode the user has.
	onDisk := []lsEpisodeItem{{
		podcastShortID: "p1",
		podcastTitle:   "Show",
		episodeName:    "An Old Episode",
		episodeShortID: "e1",
		modTime:        time.Now().Add(-72 * time.Hour),
	}}
	got := collectCatalogEpisodes(nil, onDisk, false)
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	if !got[0].Downloaded || got[0].Item == nil {
		t.Errorf("on-disk episode not marked downloaded: %+v", got[0])
	}
}

func TestCollectCatalogEpisodesSortsNewestFirst(t *testing.T) {
	t.Parallel()
	now := time.Now()
	onDisk := []lsEpisodeItem{
		{podcastShortID: "p1", episodeName: "older", modTime: now.Add(-48 * time.Hour)},
		{podcastShortID: "p1", episodeName: "newer", modTime: now.Add(-1 * time.Hour)},
	}
	got := collectCatalogEpisodes(nil, onDisk, false)
	if len(got) != 2 || got[0].Title != "newer" {
		t.Errorf("not newest first: %+v", got)
	}
}

func TestHourlyPodcastsAreHiddenByDefault(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hourly := filepath.Join(dir, "Rolling_News")
	if err := os.MkdirAll(hourly, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.PodcastConfig{}
	cfg.Frequency = &types.PodcastFrequencyInfo{Type: "hourly", EpisodesPerWeek: 169}
	if err := config.SavePodcastConfig(hourly, cfg); err != nil {
		t.Fatal(err)
	}
	if !isHourlyPodcast(hourly) {
		t.Error("an hourly podcast was not recognised")
	}

	daily := filepath.Join(dir, "Daily_Show")
	if err := os.MkdirAll(daily, 0o755); err != nil {
		t.Fatal(err)
	}
	dcfg := config.PodcastConfig{}
	dcfg.Frequency = &types.PodcastFrequencyInfo{Type: "daily", EpisodesPerWeek: 5}
	if err := config.SavePodcastConfig(daily, dcfg); err != nil {
		t.Fatal(err)
	}
	// Daily shows are the point of the listing and must survive the filter.
	if isHourlyPodcast(daily) {
		t.Error("a daily podcast was treated as hourly")
	}

	// A podcast with no cadence recorded has not been analysed and must not
	// be hidden on a guess.
	plain := filepath.Join(dir, "Unanalysed")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if isHourlyPodcast(plain) {
		t.Error("an unanalysed podcast was hidden")
	}
}

func TestCatalogKeyStripsTheDatePrefixFromFilenames(t *testing.T) {
	t.Parallel()
	// pod prefixes a downloaded file with the publication date; the feed title
	// has none. Without stripping it, every downloaded episode failed to match
	// its own catalogue entry and appeared twice in the listing.
	feed := catalogKey("p1", "Trump Rages Openly at Midterm Woes")
	file := catalogKey("p1", "2026-09-16_Trump_Rages_Openly_at_Midterm_Woes")
	if feed != file {
		t.Errorf("date prefix broke the match:\n  feed=%q\n  file=%q", feed, file)
	}
	if u := catalogKey("p1", "2026_09_16_Same_Thing"); u != catalogKey("p1", "Same Thing") {
		t.Error("underscore-separated dates not stripped")
	}
}

func TestCatalogKeyKeepsEpisodeNumbers(t *testing.T) {
	t.Parallel()
	// Digits survive, because numbering is often all that separates two
	// otherwise identical titles.
	if catalogKey("p1", "Episode 219") == catalogKey("p1", "Episode 220") {
		t.Error("episode numbers collapsed")
	}
}
