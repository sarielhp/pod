package podsite

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata golden files")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run: go test ./pkg/podsite -update)", name, err)
	}
	if string(got) != string(want) {
		t.Errorf("%s does not match golden; got:\n%s", name, got)
	}
}

func fixture() (Show, []Episode) {
	show := Show{
		Title:     "Hardcore History & Friends",
		FeedURL:   "https://dancarlin.com/feed.xml",
		Folder:    "Hardcore History",
		ImageURL:  "https://remote.example.com/art.jpg",
		CoverFile: "cover.jpg",
		CoverSrc:  "cover.jpg",
		Hostname:  "test-host",
	}
	eps := []Episode{
		{
			Filename:    "2020-09-13 - Episode 69 - Twilight & Ash.mp3",
			Title:       "Episode 69 - Twilight & Ash",
			GUID:        "guid-ep-69",
			PubDate:     "Sun, 13 Sep 2020 12:26:40 +0000",
			Description: "Ampersand & <tags> and \"quotes\".",
			DurationSec: 15155,
			SizeBytes:   24,
			ReportHref:  "2020-09-13%20-%20Episode%2069%20-%20Twilight%20%26%20Ash.report.html",
		},
		{
			Filename:    "2021-01-02 - Ep 70 - ünïcôde.mp3",
			Title:       "Ep 70 - ünïcôde",
			GUID:        "pod:ep:0badc0de",
			PubDate:     "Sat, 02 Jan 2021 00:00:00 +0000",
			Description: "Ep 70 - ünïcôde",
			SizeBytes:   2048,
		},
	}
	return show, eps
}

func TestRenderFeedGolden(t *testing.T) {
	t.Parallel()
	show, eps := fixture()
	data, err := RenderFeed(show, eps, "http://srv:8080/podcasts")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "feed_withbase.xml", data)

	data, err = RenderFeed(show, eps, "")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "feed_nobase.xml", data)
}

func TestRenderFeedRemoteImageFallback(t *testing.T) {
	t.Parallel()
	show, eps := fixture()
	show.CoverFile = ""
	data, err := RenderFeed(show, eps, "http://srv:8080/podcasts")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "feed_remoteimg.xml", data)
}

func TestRenderShowPageGolden(t *testing.T) {
	t.Parallel()
	show, eps := fixture()
	data, err := RenderShowPage(show, eps, "http://srv:8080/podcasts")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "page_withbase.html", data)

	data, err = RenderShowPage(show, eps, "")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "page_nobase.html", data)
}

func TestRenderCatalogGolden(t *testing.T) {
	t.Parallel()
	entries := []CatalogEntry{
		{Title: "Zeta Show", Folder: "Zeta Show", CoverSrc: "Zeta%20Show/cover.jpg", EpisodeCount: 3},
		{Title: "alpha show", Folder: "alpha show", CoverSrc: "https://remote.example.com/art.jpg", EpisodeCount: 0},
	}
	data, err := RenderCatalog(entries, CatalogOptions{HasOPML: true, Hostname: "test-host"})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "catalog_opml.html", data)

	data, err = RenderCatalog(entries, CatalogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "catalog_plain.html", data)
}

// RenderCatalog sorts for display; it must not reorder the caller's slice.
func TestRenderCatalogDoesNotMutateInput(t *testing.T) {
	t.Parallel()
	entries := []CatalogEntry{{Title: "Zeta"}, {Title: "alpha"}}
	if _, err := RenderCatalog(entries, CatalogOptions{}); err != nil {
		t.Fatal(err)
	}
	if entries[0].Title != "Zeta" || entries[1].Title != "alpha" {
		t.Errorf("RenderCatalog reordered the caller's slice: %v", entries)
	}
}

func TestRenderHostnamePresence(t *testing.T) {
	t.Parallel()
	entries := []CatalogEntry{{Title: "Test", Folder: "test"}}
	htmlWithHost, err := RenderCatalog(entries, CatalogOptions{Hostname: "myhost"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlWithHost), `<span class="header-hostname">myhost</span>`) {
		t.Errorf("expected hostname in catalog HTML")
	}

	htmlNoHost, err := RenderCatalog(entries, CatalogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(htmlNoHost), `<span class="header-hostname">`) {
		t.Errorf("unexpected hostname span in catalog HTML")
	}

	showWithHost := Show{Title: "Show", Hostname: "myhost"}
	showHTML, err := RenderShowPage(showWithHost, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(showHTML), `<span class="header-hostname">myhost</span>`) {
		t.Errorf("expected hostname in show HTML")
	}

	showNoHost := Show{Title: "Show"}
	showNoHostHTML, err := RenderShowPage(showNoHost, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(showNoHostHTML), `<span class="header-hostname">`) {
		t.Errorf("unexpected hostname span in show HTML without hostname")
	}
}
