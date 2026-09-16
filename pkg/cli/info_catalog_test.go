package cli

import (
	"testing"
	"time"
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
	got := collectCatalogEpisodes(nil, onDisk)
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
	got := collectCatalogEpisodes(nil, onDisk)
	if len(got) != 2 || got[0].Title != "newer" {
		t.Errorf("not newest first: %+v", got)
	}
}
