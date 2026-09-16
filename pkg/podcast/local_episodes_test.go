package podcast

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/backend"
)

func TestGeneratePodcastFeedXML(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	podDir := filepath.Join(tmpDir, "Hardcore_History")
	_ = os.MkdirAll(podDir, 0755)

	ep1 := filepath.Join(podDir, "Episode 69 - Twilight.mp3")
	_ = os.WriteFile(ep1, []byte("fake mp3 data"), 0644)

	cover := filepath.Join(podDir, "cover.jpg")
	_ = os.WriteFile(cover, []byte("fake image data"), 0644)

	sub := Subscription{
		ID:      "hh",
		Title:   "Hardcore History",
		FeedURL: "https://dancarlin.com/feed.xml",
		Folder:  "Hardcore_History",
	}

	feedEps := []backend.FeedEpisode{
		{
			Title:           "Episode 69 - Twilight",
			GUID:            "guid-ep-69",
			Description:     "A great history episode about the eastern front.",
			PublishedAt:     1600000000000,
			DurationSeconds: 15155, // ~4h 12m 35s
		},
	}

	baseURL := "http://myserver.tailscale.net:8080/podcasts"
	err := PublishPodcast(podDir, sub, baseURL, feedEps)
	if err != nil {
		t.Fatalf("PublishPodcast failed: %v", err)
	}

	feedPath := filepath.Join(podDir, "feed.xml")
	data, err := os.ReadFile(feedPath)
	if err != nil {
		t.Fatalf("failed to read feed.xml: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "<rss version=\"2.0\"") {
		t.Errorf("missing rss version tag")
	}
	if !strings.Contains(content, "<title>Hardcore History</title>") {
		t.Errorf("missing title tag")
	}
	if !strings.Contains(content, "http://myserver.tailscale.net:8080/podcasts/Hardcore_History/cover.jpg") {
		t.Errorf("missing or incorrect cover image URL: %s", content)
	}
	if !strings.Contains(content, "guid-ep-69") {
		t.Errorf("missing preserved upstream GUID: %s", content)
	}
	if !strings.Contains(content, "http://myserver.tailscale.net:8080/podcasts/Hardcore_History/Episode%2069%20-%20Twilight.mp3") {
		t.Errorf("missing or incorrect enclosure URL: %s", content)
	}

	// Verify it can be unmarshaled as valid XML. Parsed with a struct local to
	// the test rather than the renderer's own types, so the assertion checks the
	// wire format instead of re-reading podsite's marshaling decisions.
	var parsedDoc struct {
		Channel struct {
			Items []struct {
				GUID string `xml:"guid"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(data, &parsedDoc); err != nil {
		t.Fatalf("feed.xml is not valid XML: %v", err)
	}
	if len(parsedDoc.Channel.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(parsedDoc.Channel.Items))
	}
	item := parsedDoc.Channel.Items[0]
	if !strings.Contains(content, "<itunes:image href=\"http://myserver.tailscale.net:8080/podcasts/Hardcore_History/cover.jpg\"") {
		t.Errorf("channel <itunes:image> missing from XML:\n%s", content)
	}
	if !strings.Contains(content, "<url>http://myserver.tailscale.net:8080/podcasts/Hardcore_History/cover.jpg</url>") {
		t.Errorf("channel <image><url> missing from XML:\n%s", content)
	}
	if item.GUID != "guid-ep-69" {
		t.Errorf("expected GUID guid-ep-69, got %s", item.GUID)
	}
}

func TestParseRSSFeedExtractsImage(t *testing.T) {
	t.Parallel()
	itunesXML := []byte(`<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
		<channel>
			<title>Itunes Show</title>
			<itunes:image href="https://example.com/itunes_cover.jpg"/>
			<item>
				<title>Ep 1</title>
				<guid>g1</guid>
				<enclosure url="https://example.com/1.mp3" type="audio/mpeg"/>
			</item>
		</channel>
	</rss>`)
	doc, err := parseRSSFeed(itunesXML)
	if err != nil {
		t.Fatalf("parseRSSFeed failed: %v", err)
	}
	if doc.ImageURL != "https://example.com/itunes_cover.jpg" {
		t.Errorf("expected itunes image url, got %q", doc.ImageURL)
	}

	rss20XML := []byte(`<rss version="2.0">
		<channel>
			<title>RSS2 Show</title>
			<image>
				<url>https://example.com/rss2_cover.png</url>
				<title>RSS2 Show</title>
				<link>https://example.com</link>
			</image>
			<item>
				<title>Ep 1</title>
				<guid>g1</guid>
				<enclosure url="https://example.com/1.mp3" type="audio/mpeg"/>
			</item>
		</channel>
	</rss>`)
	doc2, err := parseRSSFeed(rss20XML)
	if err != nil {
		t.Fatalf("parseRSSFeed rss20 failed: %v", err)
	}
	if doc2.ImageURL != "https://example.com/rss2_cover.png" {
		t.Errorf("expected rss2 image url, got %q", doc2.ImageURL)
	}
}

func TestEnsurePodcastCoverCopiesDetailsCache(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	podDir := filepath.Join(tmpDir, "TestShow")
	detailsDir := filepath.Join(podDir, ".cache", "details")
	_ = os.MkdirAll(detailsDir, 0755)

	cachedCover := filepath.Join(detailsDir, "cover.jpg")
	_ = os.WriteFile(cachedCover, []byte("cached image data"), 0644)

	sub := Subscription{Title: "TestShow", Folder: "TestShow"}
	resolved := ensurePodcastCover(podDir, &sub)
	target := filepath.Join(podDir, "cover.jpg")
	if resolved != target {
		t.Fatalf("expected resolved cover at %s, got %s", target, resolved)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "cached image data" {
		t.Errorf("unexpected target cover data: %s, err: %v", string(data), err)
	}
}
