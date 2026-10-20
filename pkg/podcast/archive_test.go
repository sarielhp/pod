package podcast

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/config"
	"pod/pkg/util"
)

func TestArchiveManifestLoadSave(t *testing.T) {
	t.Parallel()
	podDir := t.TempDir()

	m, err := LoadArchiveManifest(podDir)
	if err != nil {
		t.Fatalf("LoadArchiveManifest on empty dir: %v", err)
	}
	if len(m.Episodes) != 0 {
		t.Fatalf("expected 0 episodes, got %d", len(m.Episodes))
	}

	ep1 := ArchivedEpisode{
		Filename:    "2026-01-01_ep1.mp3",
		Title:       "Episode 1",
		GUID:        "guid-1",
		PublishedAt: 1000,
		SizeBytes:   1024,
	}
	ep2 := ArchivedEpisode{
		Filename:    "2026-01-02_ep2.mp3",
		Title:       "Episode 2",
		GUID:        "guid-2",
		PublishedAt: 2000,
		SizeBytes:   2048,
	}

	if err := AddOrUpdateArchivedEpisode(podDir, ep1); err != nil {
		t.Fatalf("AddOrUpdateArchivedEpisode ep1: %v", err)
	}
	if err := AddOrUpdateArchivedEpisode(podDir, ep2); err != nil {
		t.Fatalf("AddOrUpdateArchivedEpisode ep2: %v", err)
	}

	loaded, err := LoadArchiveManifest(podDir)
	if err != nil {
		t.Fatalf("re-load archive manifest: %v", err)
	}
	if len(loaded.Episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(loaded.Episodes))
	}
	if loaded.Episodes[0].Filename != "2026-01-02_ep2.mp3" {
		t.Fatalf("expected ep2 first (newest publishedAt), got %s", loaded.Episodes[0].Filename)
	}

	// Update existing ep1
	ep1Updated := ep1
	ep1Updated.Title = "Episode 1 Updated"
	if err := AddOrUpdateArchivedEpisode(podDir, ep1Updated); err != nil {
		t.Fatalf("update ep1: %v", err)
	}

	loadedAgain, _ := LoadArchiveManifest(podDir)
	if len(loadedAgain.Episodes) != 2 {
		t.Fatalf("expected 2 episodes after update, got %d", len(loadedAgain.Episodes))
	}
	if loadedAgain.Episodes[1].Title != "Episode 1 Updated" {
		t.Fatalf("expected updated title, got %s", loadedAgain.Episodes[1].Title)
	}
}

func TestTransferEpisodeToArchivePreservesSidecarsOnSSD(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()
	podDir := filepath.Join(baseDir, "The_Show")
	archiveDir := filepath.Join(baseDir, "hdd_archive")
	if err := os.MkdirAll(podDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		t.Fatal(err)
	}

	mp3Path := filepath.Join(podDir, "2026-01-10_test.mp3")
	precutPath := mp3Path + ".precut"
	transcriptJSON := filepath.Join(podDir, "2026-01-10_test.transcript.json")
	transcriptTXT := filepath.Join(podDir, "2026-01-10_test.txt")
	reportHTML := filepath.Join(podDir, "2026-01-10_test.report.html")

	_ = os.WriteFile(mp3Path, []byte("fake-mp3-audio-data"), 0644)
	_ = os.WriteFile(precutPath, []byte("fake-precut"), 0644)
	_ = os.WriteFile(transcriptJSON, []byte(`{"segments":[]}`), 0644)
	_ = os.WriteFile(transcriptTXT, []byte("transcript text"), 0644)
	_ = os.WriteFile(reportHTML, []byte("<html>report</html>"), 0644)

	meta, freed, err := TransferEpisodeToArchive(mp3Path, podDir, archiveDir, false)
	if err != nil {
		t.Fatalf("TransferEpisodeToArchive failed: %v", err)
	}
	if freed <= 0 {
		t.Errorf("expected positive bytes freed, got %d", freed)
	}
	if meta.Filename != "2026-01-10_test.mp3" {
		t.Errorf("expected filename 2026-01-10_test.mp3, got %s", meta.Filename)
	}

	// Verify MP3 and precut removed from SSD
	if util.FileExists(mp3Path) {
		t.Errorf("expected MP3 to be removed from SSD podDir")
	}
	if util.FileExists(precutPath) {
		t.Errorf("expected precut to be removed from SSD podDir")
	}

	// Verify sidecars KEPT on SSD for fast search
	if !util.FileExists(transcriptJSON) {
		t.Errorf("transcript JSON must remain on SSD")
	}
	if !util.FileExists(transcriptTXT) {
		t.Errorf("transcript TXT must remain on SSD")
	}
	if !util.FileExists(reportHTML) {
		t.Errorf("report HTML must remain on SSD")
	}

	// Verify MP3 and mirrored sidecars exist in HDD mirror
	hddShowDir := filepath.Join(archiveDir, "The_Show")
	if !util.FileExists(filepath.Join(hddShowDir, "2026-01-10_test.mp3")) {
		t.Errorf("archived MP3 missing on HDD")
	}
	if !util.FileExists(filepath.Join(hddShowDir, "2026-01-10_test.transcript.json")) {
		t.Errorf("mirrored transcript JSON missing on HDD")
	}
	if !util.FileExists(filepath.Join(hddShowDir, "2026-01-10_test.txt")) {
		t.Errorf("mirrored transcript TXT missing on HDD")
	}

	// Verify archive.json on SSD recorded the episode
	m, err := LoadArchiveManifest(podDir)
	if err != nil || len(m.Episodes) != 1 {
		t.Fatalf("expected 1 archived episode in manifest, got %v (err: %v)", m, err)
	}
	if m.Episodes[0].Filename != "2026-01-10_test.mp3" {
		t.Errorf("manifest filename = %s", m.Episodes[0].Filename)
	}
}

func TestTransferEpisodeToArchive_DryRunLeavesFilesIntact(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()
	podDir := filepath.Join(baseDir, "Show")
	archiveDir := filepath.Join(baseDir, "archive")
	_ = os.MkdirAll(podDir, 0755)

	mp3Path := filepath.Join(podDir, "ep.mp3")
	_ = os.WriteFile(mp3Path, []byte("data"), 0644)

	meta, freed, err := TransferEpisodeToArchive(mp3Path, podDir, archiveDir, true)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if freed != int64(len("data")) {
		t.Errorf("expected %d freed, got %d", len("data"), freed)
	}
	if meta.Filename != "ep.mp3" {
		t.Errorf("meta.Filename = %s", meta.Filename)
	}
	if !util.FileExists(mp3Path) {
		t.Errorf("dry-run must not delete source file")
	}
	if util.FileExists(filepath.Join(archiveDir, "Show", "ep.mp3")) {
		t.Errorf("dry-run must not write to archive dir")
	}
}

func TestApplyPodcastKeepPolicyWithArchive(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()
	podDir := filepath.Join(baseDir, "The_Show")
	archiveDir := filepath.Join(baseDir, "hdd")
	_ = os.MkdirAll(podDir, 0755)
	_ = os.MkdirAll(archiveDir, 0755)

	// Episode 1: 40 days old (expired under 30d policy)
	epOld := filepath.Join(podDir, "2026-01-01_old.mp3")
	_ = os.WriteFile(epOld, []byte("old-audio"), 0644)
	oldTime := time.Now().AddDate(0, 0, -40)
	_ = os.Chtimes(epOld, oldTime, oldTime)

	// Episode 2: 5 days old (fresh, keep)
	epFresh := filepath.Join(podDir, "2026-02-10_fresh.mp3")
	_ = os.WriteFile(epFresh, []byte("fresh-audio"), 0644)
	freshTime := time.Now().AddDate(0, 0, -5)
	_ = os.Chtimes(epFresh, freshTime, freshTime)

	cfg := config.PodcastConfig{
		AutoCleanupDays: 30,
	}
	autoCleanupTrue := true
	cfg.AutoCleanup = &autoCleanupTrue

	archiveOpts := ArchiveOptions{
		Enabled: true,
		Dir:     archiveDir,
	}

	res, err := ApplyPodcastKeepPolicyWithArchive(podDir, "The Show", cfg, archiveOpts, time.Now(), false)
	if err != nil {
		t.Fatalf("ApplyPodcastKeepPolicyWithArchive failed: %v", err)
	}
	if res.ExpiredEpisodes != 1 {
		t.Errorf("expected 1 expired, got %d", res.ExpiredEpisodes)
	}
	if res.ArchivedEpisodes != 1 {
		t.Errorf("expected 1 archived, got %d", res.ArchivedEpisodes)
	}
	if res.DeletedEpisodes != 0 {
		t.Errorf("expected 0 deleted, got %d", res.DeletedEpisodes)
	}

	// Verify old episode was moved to archive and removed from SSD
	if util.FileExists(epOld) {
		t.Errorf("expired episode should be removed from SSD podDir")
	}
	if !util.FileExists(filepath.Join(archiveDir, "The_Show", "2026-01-01_old.mp3")) {
		t.Errorf("expired episode must exist in HDD archive")
	}

	// Verify fresh episode untouched
	if !util.FileExists(epFresh) {
		t.Errorf("fresh episode must remain on SSD")
	}

	// Verify archive.json on SSD holds the archived episode
	m, err := LoadArchiveManifest(podDir)
	if err != nil || len(m.Episodes) != 1 {
		t.Fatalf("expected 1 episode in archive.json, got %v (err: %v)", m, err)
	}
	if m.Episodes[0].Filename != "2026-01-01_old.mp3" {
		t.Errorf("expected 2026-01-01_old.mp3 in archive.json, got %s", m.Episodes[0].Filename)
	}
}

func TestPublishPodcastWithArchiveZeroSpin(t *testing.T) {
	t.Parallel()
	podDir := t.TempDir()

	// 1. One active local episode on SSD
	activeMP3 := filepath.Join(podDir, "2026-03-01_active.mp3")
	_ = os.WriteFile(activeMP3, []byte("active-content"), 0644)

	// 2. Archive manifest on SSD (describes cold episode, HDD is NOT mounted)
	archivedEp := ArchivedEpisode{
		Filename:    "2026-01-10_cold.mp3",
		Title:       "Cold Episode",
		GUID:        "guid-cold-1",
		PubDate:     "Sat, 10 Jan 2026 12:00:00 +0000",
		PublishedAt: 1768046400000,
		DurationSec: 3600,
		SizeBytes:   45000000,
		Description: "Notes for cold episode",
	}
	if err := AddOrUpdateArchivedEpisode(podDir, archivedEp); err != nil {
		t.Fatalf("AddOrUpdateArchivedEpisode: %v", err)
	}

	sub := Subscription{
		Title:  "Test Show",
		Folder: "Test_Show",
	}

	baseURL := "http://myhost:8080/podcasts"
	archiveBaseURL := "http://myhost:8080/archive"

	// Publish podcast with archive - NO HDD access needed!
	if err := PublishPodcastWithArchive(podDir, sub, baseURL, archiveBaseURL, nil); err != nil {
		t.Fatalf("PublishPodcastWithArchive failed: %v", err)
	}

	// 1. Verify feed.xml
	feedPath := filepath.Join(podDir, "feed.xml")
	feedData, err := os.ReadFile(feedPath)
	if err != nil {
		t.Fatalf("read feed.xml: %v", err)
	}

	var rss struct {
		Channel struct {
			Items []struct {
				Title     string `xml:"title"`
				Enclosure struct {
					URL    string `xml:"url,attr"`
					Length int64  `xml:"length,attr"`
				} `xml:"enclosure"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(feedData, &rss); err != nil {
		t.Fatalf("unmarshal feed.xml: %v", err)
	}

	if len(rss.Channel.Items) != 2 {
		t.Fatalf("expected 2 items in feed.xml (1 active, 1 archived), got %d", len(rss.Channel.Items))
	}

	var foundArchivedFeed bool
	for _, item := range rss.Channel.Items {
		if item.Title == "Cold Episode" {
			foundArchivedFeed = true
			wantURL := "http://myhost:8080/archive/Test_Show/2026-01-10_cold.mp3"
			if item.Enclosure.URL != wantURL {
				t.Errorf("archived enclosure url = %q, want %q", item.Enclosure.URL, wantURL)
			}
			if item.Enclosure.Length != 45000000 {
				t.Errorf("archived enclosure length = %d, want 45000000", item.Enclosure.Length)
			}
		}
	}
	if !foundArchivedFeed {
		t.Errorf("feed.xml missing archived episode item")
	}

	// 2. Verify index.html
	indexPath := filepath.Join(podDir, "index.html")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	htmlStr := string(indexData)

	if !strings.Contains(htmlStr, "Cold Episode") {
		t.Errorf("index.html missing Cold Episode title")
	}
	if !strings.Contains(htmlStr, "badge ep-archived") {
		t.Errorf("index.html missing badge ep-archived for cold episode")
	}
	if !strings.Contains(htmlStr, "http://myhost:8080/archive/Test_Show/2026-01-10_cold.mp3") {
		t.Errorf("index.html audio source missing archive URL")
	}
}

func TestPublishCatalogIncludesArchivedEpisodes(t *testing.T) {
	t.Parallel()
	podcastsDir := t.TempDir()
	showDir := filepath.Join(podcastsDir, "My_Show")
	_ = os.MkdirAll(showDir, 0755)

	// 1 active file
	_ = os.WriteFile(filepath.Join(showDir, "active.mp3"), []byte("active"), 0644)

	// 2 archived files in archive.json
	arch1 := ArchivedEpisode{Filename: "arch1.mp3", Title: "A1", PublishedAt: 100}
	arch2 := ArchivedEpisode{Filename: "arch2.mp3", Title: "A2", PublishedAt: 200}
	_ = AddOrUpdateArchivedEpisode(showDir, arch1)
	_ = AddOrUpdateArchivedEpisode(showDir, arch2)

	subs := []Subscription{
		{Title: "My Show", Folder: "My_Show"},
	}

	if err := PublishCatalog(podcastsDir, subs); err != nil {
		t.Fatalf("PublishCatalog failed: %v", err)
	}

	catalogHTML, err := os.ReadFile(filepath.Join(podcastsDir, "index.html"))
	if err != nil {
		t.Fatalf("read catalog index.html: %v", err)
	}

	// Total should be 1 active + 2 archived = 3 episode(s)
	if !strings.Contains(string(catalogHTML), "3 episode(s)") {
		t.Errorf("catalog HTML should report 3 episode(s), got:\n%s", string(catalogHTML))
	}
}
