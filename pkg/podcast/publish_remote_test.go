package podcast

import (
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/podsite"
)

func TestAppendRemoteEpisodesPublishesTheWholeRun(t *testing.T) {
	t.Parallel()
	// A show with nothing downloaded used to publish an empty feed, so
	// subscribing locally gave strictly less than subscribing upstream.
	feed := []backend.FeedEpisode{
		{Title: "Episode One", PublishedAt: 1000, GUID: "g1", EnclosureURL: "https://cdn/1.mp3"},
		{Title: "Episode Two", PublishedAt: 2000, GUID: "g2", EnclosureURL: "https://cdn/2.mp3"},
	}
	got := appendRemoteEpisodes(nil, nil, feed)
	if len(got) != 2 {
		t.Fatalf("got %d episodes, want 2", len(got))
	}
	for _, e := range got {
		if e.Local() {
			t.Errorf("episode %q claimed to be local", e.Title)
		}
		if e.RemoteURL == "" {
			t.Errorf("episode %q has no audio URL", e.Title)
		}
	}
}

func TestAppendRemoteEpisodesPrefersTheLocalCopy(t *testing.T) {
	t.Parallel()
	// A downloaded episode is served from here, ad-free; it must not also be
	// published pointing upstream.
	local := []podsite.Episode{{Title: "Episode One", Filename: "2026-01-01_Episode_One.mp3"}}
	meta := []LocalEpisodeMeta{{
		EpisodeFile: EpisodeFile{Title: "Episode One", PublishedAt: 1000},
		GUID:        "g1",
	}}
	feed := []backend.FeedEpisode{
		{Title: "Episode One", PublishedAt: 1000, GUID: "g1", EnclosureURL: "https://cdn/1.mp3"},
		{Title: "Episode Two", PublishedAt: 2000, GUID: "g2", EnclosureURL: "https://cdn/2.mp3"},
	}
	got := appendRemoteEpisodes(local, meta, feed)
	if len(got) != 2 {
		t.Fatalf("got %d episodes, want 2 (one local, one remote)", len(got))
	}
	for _, e := range got {
		if e.Title == "Episode One" && !e.Local() {
			t.Error("the downloaded episode was published pointing upstream")
		}
	}
}

func TestAppendRemoteEpisodesMatchesAFilenameDerivedTitle(t *testing.T) {
	t.Parallel()
	// A local episode whose title could not be matched to the feed carries a
	// title derived from its filename. It must still dedupe, or the episode
	// is published twice — once from each side.
	meta := []LocalEpisodeMeta{{
		EpisodeFile: EpisodeFile{Title: "2026-01-01_Episode_One", PublishedAt: 1000},
	}}
	local := []podsite.Episode{{Title: "2026-01-01_Episode_One"}}
	feed := []backend.FeedEpisode{
		{Title: "Episode One", PublishedAt: 1000, GUID: "g1", EnclosureURL: "https://cdn/1.mp3"},
	}
	if got := appendRemoteEpisodes(local, meta, feed); len(got) != 1 {
		t.Errorf("episode published twice: %+v", got)
	}
}

func TestAppendRemoteEpisodesSkipsEntriesWithNoAudio(t *testing.T) {
	t.Parallel()
	// The retained history predates enclosure URLs being kept, so older
	// entries have none. Publishing them would produce items nothing can play.
	feed := []backend.FeedEpisode{
		{Title: "No URL", PublishedAt: 1000},
		{Title: "Has URL", PublishedAt: 2000, EnclosureURL: "https://cdn/2.mp3"},
	}
	got := appendRemoteEpisodes(nil, nil, feed)
	if len(got) != 1 || got[0].Title != "Has URL" {
		t.Errorf("got %+v, want only the playable episode", got)
	}
}

func TestAppendRemoteEpisodesOrdersByInstantNotByFormattedDate(t *testing.T) {
	t.Parallel()
	// The defect this exists to prevent: ordering on the RFC1123 string put
	// "Wed, 29 Apr" above "Wed, 16 Sep", because the month is text. The feed
	// interleaved years and the newest episode landed tenth, so AntennaPod
	// never showed the day's episode.
	ms := func(y int, m time.Month, d int) int64 {
		return time.Date(y, m, d, 9, 0, 0, 0, time.UTC).UnixMilli()
	}
	feed := []backend.FeedEpisode{
		{Title: "April", PublishedAt: ms(2026, time.April, 29), EnclosureURL: "https://cdn/a.mp3"},
		{Title: "September", PublishedAt: ms(2026, time.September, 16), EnclosureURL: "https://cdn/s.mp3"},
		{Title: "August", PublishedAt: ms(2026, time.August, 26), EnclosureURL: "https://cdn/g.mp3"},
	}
	got := appendRemoteEpisodes(nil, nil, feed)
	if len(got) != 3 {
		t.Fatalf("got %d episodes", len(got))
	}
	want := []string{"September", "August", "April"}
	for i, title := range want {
		if got[i].Title != title {
			t.Fatalf("position %d is %q, want %q (order: %s, %s, %s)",
				i, got[i].Title, title, got[0].Title, got[1].Title, got[2].Title)
		}
	}
}

func TestAppendRemoteEpisodesOrdersLocalAndRemoteTogether(t *testing.T) {
	t.Parallel()
	// A downloaded episode must take its place by date among the passthrough
	// ones, not sit in whichever half it came from.
	ms := func(d int) int64 { return time.Date(2026, time.September, d, 9, 0, 0, 0, time.UTC).UnixMilli() }
	local := []podsite.Episode{{Title: "Local 15th", PublishedAt: ms(15), Filename: "local.mp3"}}
	meta := []LocalEpisodeMeta{{EpisodeFile: EpisodeFile{Title: "Local 15th", PublishedAt: ms(15)}}}
	feed := []backend.FeedEpisode{
		{Title: "Remote 16th", PublishedAt: ms(16), EnclosureURL: "https://cdn/16.mp3"},
		{Title: "Remote 14th", PublishedAt: ms(14), EnclosureURL: "https://cdn/14.mp3"},
	}
	got := appendRemoteEpisodes(local, meta, feed)
	want := []string{"Remote 16th", "Local 15th", "Remote 14th"}
	for i, title := range want {
		if got[i].Title != title {
			t.Errorf("position %d is %q, want %q", i, got[i].Title, title)
		}
	}
}
