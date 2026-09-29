package podcast

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/episode"
)

func writeAudio(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func feedEpisode(guid, title string, published time.Time) backend.FeedEpisode {
	return backend.FeedEpisode{GUID: guid, Title: title, PublishedAt: published.UnixMilli(), EnclosureURL: "https://example.test/" + guid + ".mp3"}
}

var pub = time.Date(2026, 9, 28, 6, 0, 0, 0, time.UTC)

func TestRecordedIdentityFindsAFileWhateverItIsCalled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fe := feedEpisode("guid-123", "A Title That The File Name Does Not Mention", pub)
	path := writeAudio(t, dir, "x7f3a.mp3")
	if err := RecordFeedEpisode(path, fe, "https://example.test/feed.xml"); err != nil {
		t.Fatal(err)
	}

	got, ok := NewLocalEpisodes(dir).Find(fe)
	if !ok || got != path {
		t.Fatalf("the episode should be found by its GUID, got %q %v", got, ok)
	}
	if _, ok := NewLocalEpisodes(dir).Find(feedEpisode("other-guid", "Another Title", pub)); ok {
		t.Error("a different episode must not match")
	}
}

func TestEpisodesDownloadedBeforeIdentitiesAreStillFoundByName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fe := feedEpisode("guid-old", "The Rise and Fall of Rome", pub)
	path := writeAudio(t, dir, FormatEpisodeFilename(pub, "", fe.Title))
	got, ok := NewLocalEpisodes(dir).Find(fe)
	if !ok || got != path {
		t.Fatalf("a legacy file with no identity must still match by its name, got %q %v", got, ok)
	}
}

func TestIdentityOutranksAMisleadingName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fe := feedEpisode("guid-a", "Episode A", pub)
	other := feedEpisode("guid-b", "Episode B", pub)
	// A file named for episode B that is really episode A.
	path := writeAudio(t, dir, FormatEpisodeFilename(pub, "", other.Title))
	if err := RecordFeedEpisode(path, fe, ""); err != nil {
		t.Fatal(err)
	}
	ix := NewLocalEpisodes(dir)
	if got, ok := ix.Find(fe); !ok || got != path {
		t.Errorf("episode A is in that file, got %q %v", got, ok)
	}
	if _, ok := ix.Find(other); ok {
		t.Error("the name says B, but the recorded identity says the file is A, and identity wins")
	}
}

func TestAFeedWithoutGUIDsGetsStableSyntheticOnes(t *testing.T) {
	t.Parallel()
	fe := backend.FeedEpisode{Title: "No GUID Here", PublishedAt: pub.UnixMilli(), EnclosureURL: "https://example.test/a.mp3"}
	a, b := IdentityOfFeedEpisode(fe, "f"), IdentityOfFeedEpisode(fe, "f")
	if !a.Synthetic || a.GUID != b.GUID || a.GUID == "" {
		t.Errorf("a missing GUID must be replaced by a stable synthetic one: %+v %+v", a, b)
	}
	fe.EnclosureURL = "https://example.test/b.mp3"
	if IdentityOfFeedEpisode(fe, "f").GUID == a.GUID {
		t.Error("different audio must not share a synthetic GUID")
	}

	dir := t.TempDir()
	path := writeAudio(t, dir, "k1.mp3")
	if err := RecordFeedEpisode(path, fe, ""); err != nil {
		t.Fatal(err)
	}
	if got, ok := NewLocalEpisodes(dir).Find(fe); !ok || got != path {
		t.Errorf("a GUID-less episode should be found through its synthetic GUID, got %q %v", got, ok)
	}
}

func TestTheDisplayTitleComesFromTheRecordedIdentity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeAudio(t, dir, "x7f3a.mp3")
	if got := episode.EpisodeTitleFromPath(path); got != "x7f3a" {
		t.Errorf("with no identity the title falls back to the name, got %q", got)
	}
	if err := RecordFeedEpisode(path, feedEpisode("g", "  The Real Title  ", pub), ""); err != nil {
		t.Fatal(err)
	}
	if got := episode.EpisodeTitleFromPath(path); got != "The Real Title" {
		t.Errorf("want the recorded title, got %q", got)
	}
	st := episode.GetOrCreateEpisodeStatus(path)
	if st.PublishedAt == "" || st.PublicationSource != "feed" {
		t.Errorf("the publication date should be recorded too: %+v", st)
	}
}

func TestNewEpisodeFilenameIsShortOpaqueAndStable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	long := feedEpisode("guid-long", strings.Repeat("ש", 200), pub)
	name := NewEpisodeFilename(dir, long)
	if len(name) != len("2026-09-28_")+10+len(".mp3") {
		t.Errorf("want a fixed short name whatever the title, got %q (%d bytes)", name, len(name))
	}
	if !strings.HasPrefix(name, "2026-09-28_") || strings.Contains(name, "ש") {
		t.Errorf("date prefix and no title expected, got %q", name)
	}
	if again := NewEpisodeFilename(dir, long); again != name {
		t.Errorf("the same episode must always get the same name: %q vs %q", name, again)
	}
	if other := NewEpisodeFilename(dir, feedEpisode("guid-other", "x", pub)); other == name {
		t.Error("different episodes must get different names")
	}
}

func TestNewEpisodeFilenameAvoidsAnotherEpisodeAndReusesItsOwn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fe := feedEpisode("guid-a", "A", pub)
	first := NewEpisodeFilename(dir, fe)

	// A different episode already sitting on that name forces a longer code.
	squatter := writeAudio(t, dir, first)
	if err := RecordFeedEpisode(squatter, feedEpisode("guid-b", "B", pub), ""); err != nil {
		t.Fatal(err)
	}
	second := NewEpisodeFilename(dir, fe)
	if second == first || len(second) <= len(first) {
		t.Errorf("a name held by another episode must be avoided with a longer code: %q then %q", first, second)
	}

	// The episode's own file is reused, so a re-download does not pile up copies.
	own := writeAudio(t, dir, second)
	if err := RecordFeedEpisode(own, fe, ""); err != nil {
		t.Fatal(err)
	}
	if third := NewEpisodeFilename(dir, fe); third != second {
		t.Errorf("an episode must keep its own name: %q vs %q", third, second)
	}
}

func TestNewEpisodeFilenameWithoutADateHasNoPrefix(t *testing.T) {
	t.Parallel()
	fe := backend.FeedEpisode{GUID: "g", Title: "Undated"}
	name := NewEpisodeFilename(t.TempDir(), fe)
	if strings.Contains(name, "_") || len(name) != 10+len(".mp3") {
		t.Errorf("an episode with no date is named by its code alone, got %q", name)
	}
}
