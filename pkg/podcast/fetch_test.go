package podcast

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/types"
)

func mockFeedServer(title string, episodes []struct{ title, pubDate, url string }) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		items := ""
		for _, ep := range episodes {
			items += fmt.Sprintf("<item><title>%s</title><pubDate>%s</pubDate><enclosure url=\"%s\" type=\"audio/mpeg\"/></item>\n",
				ep.title, ep.pubDate, ep.url)
		}
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>` + title + `</title>
` + items + `</channel></rss>`))
	}))
}

func TestPlanFetchLatestAlreadyDownloadedDoesNothing(t *testing.T) {
	t.Parallel()
	now := time.Now()
	d1 := now.Add(-3 * time.Hour).Format(time.RFC1123Z)
	d2 := now.Add(-2 * time.Hour).Format(time.RFC1123Z)
	d3 := now.Add(-1 * time.Hour).Format(time.RFC1123Z)

	ts := mockFeedServer("Tech News", []struct{ title, pubDate, url string }{
		{"Ep 1", d1, "http://example.com/1.mp3"},
		{"Ep 2", d2, "http://example.com/2.mp3"},
		{"Ep 3", d3, "http://example.com/3.mp3"},
	})
	defer ts.Close()

	root := t.TempDir()
	showDir := filepath.Join(root, "Tech News")
	if err := os.MkdirAll(showDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Only Ep 3 (the latest) is on disk. Older episodes 1 and 2 are NOT on disk.
	if err := os.WriteFile(filepath.Join(showDir, "Ep 3.mp3"), []byte("mp3data"), 0644); err != nil {
		t.Fatal(err)
	}

	subs := []Subscription{
		{ID: "tn", Title: "Tech News", Folder: "Tech News", FeedURL: ts.URL},
	}

	lib := Open(Config{PodcastsDir: root}, nil, nil)
	plans := lib.PlanFetch(subs, FetchOptions{Count: 1}, nil)

	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if len(plans[0].ToDownload) != 0 {
		t.Fatalf("expected 0 episodes to download since latest is on disk, got %d (never backfill!)", len(plans[0].ToDownload))
	}
}

func TestPlanFetchLatestNotDownloaded(t *testing.T) {
	t.Parallel()
	now := time.Now()
	d1 := now.Add(-2 * time.Hour).Format(time.RFC1123Z)
	d2 := now.Add(-1 * time.Hour).Format(time.RFC1123Z)

	ts := mockFeedServer("Science Pod", []struct{ title, pubDate, url string }{
		{"Ep 1", d1, "http://example.com/1.mp3"},
		{"Ep 2", d2, "http://example.com/2.mp3"},
	})
	defer ts.Close()

	root := t.TempDir()
	showDir := filepath.Join(root, "Science Pod")
	if err := os.MkdirAll(showDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Ep 1 is downloaded, Ep 2 is NOT
	if err := os.WriteFile(filepath.Join(showDir, "Ep 1.mp3"), []byte("mp3data"), 0644); err != nil {
		t.Fatal(err)
	}

	subs := []Subscription{
		{ID: "sci", Title: "Science Pod", Folder: "Science Pod", FeedURL: ts.URL},
	}

	lib := Open(Config{PodcastsDir: root}, nil, nil)
	plans := lib.PlanFetch(subs, FetchOptions{Count: 1}, nil)

	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if len(plans[0].ToDownload) != 1 {
		t.Fatalf("expected 1 episode to download, got %d", len(plans[0].ToDownload))
	}
	if plans[0].ToDownload[0].Title != "Ep 2" {
		t.Errorf("expected Ep 2 to be downloaded, got %q", plans[0].ToDownload[0].Title)
	}
}

func TestPlanFetchCountN(t *testing.T) {
	t.Parallel()
	now := time.Now()
	d1 := now.Add(-4 * time.Hour).Format(time.RFC1123Z)
	d2 := now.Add(-3 * time.Hour).Format(time.RFC1123Z)
	d3 := now.Add(-2 * time.Hour).Format(time.RFC1123Z)
	d4 := now.Add(-1 * time.Hour).Format(time.RFC1123Z)

	ts := mockFeedServer("History Show", []struct{ title, pubDate, url string }{
		{"Ep 1", d1, "http://example.com/1.mp3"},
		{"Ep 2", d2, "http://example.com/2.mp3"},
		{"Ep 3", d3, "http://example.com/3.mp3"},
		{"Ep 4", d4, "http://example.com/4.mp3"},
	})
	defer ts.Close()

	root := t.TempDir()
	showDir := filepath.Join(root, "History Show")
	if err := os.MkdirAll(showDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Ep 4 is on disk, Ep 3, 2, 1 are NOT
	if err := os.WriteFile(filepath.Join(showDir, "Ep 4.mp3"), []byte("mp3data"), 0644); err != nil {
		t.Fatal(err)
	}

	subs := []Subscription{
		{ID: "hist", Title: "History Show", Folder: "History Show", FeedURL: ts.URL},
	}

	lib := Open(Config{PodcastsDir: root}, nil, nil)
	// With Count = 2, inspects latest 2 (Ep 3 and Ep 4). Ep 4 is on disk, so only Ep 3 should download.
	plans := lib.PlanFetch(subs, FetchOptions{Count: 2}, nil)

	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if len(plans[0].ToDownload) != 1 {
		t.Fatalf("expected 1 episode to download, got %d", len(plans[0].ToDownload))
	}
	if plans[0].ToDownload[0].Title != "Ep 3" {
		t.Errorf("expected Ep 3 to be downloaded, got %q", plans[0].ToDownload[0].Title)
	}

	// Now also put Ep 3 on disk. Ep 1 and Ep 2 are still missing.
	if err := os.WriteFile(filepath.Join(showDir, "Ep 3.mp3"), []byte("mp3data"), 0644); err != nil {
		t.Fatal(err)
	}
	plans2 := lib.PlanFetch(subs, FetchOptions{Count: 2}, nil)
	if len(plans2[0].ToDownload) != 0 {
		t.Fatalf("expected 0 episodes to download since latest 2 are on disk (never backfill older!), got %d", len(plans2[0].ToDownload))
	}
}

func TestPlanFetchSkipsHourlyByDefault(t *testing.T) {
	t.Parallel()
	now := time.Now()
	d1 := now.Add(-1 * time.Hour).Format(time.RFC1123Z)

	ts1 := mockFeedServer("Daily Show", []struct{ title, pubDate, url string }{
		{"Ep 1", d1, "http://example.com/daily.mp3"},
	})
	defer ts1.Close()

	ts2 := mockFeedServer("Hourly News", []struct{ title, pubDate, url string }{
		{"Bulletin 1", d1, "http://example.com/hourly.mp3"},
	})
	defer ts2.Close()

	root := t.TempDir()
	dailyDir := filepath.Join(root, "Daily Show")
	hourlyDir := filepath.Join(root, "Hourly News")
	_ = os.MkdirAll(dailyDir, 0755)
	_ = os.MkdirAll(hourlyDir, 0755)

	_ = config.SavePodcastConfig(hourlyDir, config.PodcastConfig{
		Frequency: &types.PodcastFrequencyInfo{
			Type: string(backend.CadenceHourly),
		},
	})

	subs := []Subscription{
		{ID: "daily", Title: "Daily Show", Folder: "Daily Show", FeedURL: ts1.URL},
		{ID: "hourly", Title: "Hourly News", Folder: "Hourly News", FeedURL: ts2.URL},
	}

	lib := Open(Config{PodcastsDir: root}, nil, nil)

	// By default (IncludeHourly = false): Hourly News must be skipped!
	plansDefault := lib.PlanFetch(subs, FetchOptions{Count: 1, IncludeHourly: false}, nil)
	if len(plansDefault) != 1 {
		t.Fatalf("expected 1 plan (skipping hourly), got %d", len(plansDefault))
	}
	if plansDefault[0].Sub.ID != "daily" {
		t.Errorf("expected daily show, got %q", plansDefault[0].Sub.ID)
	}

	// With IncludeHourly = true: Hourly News must be included!
	plansHourly := lib.PlanFetch(subs, FetchOptions{Count: 1, IncludeHourly: true}, nil)
	if len(plansHourly) != 2 {
		t.Fatalf("expected 2 plans (including hourly), got %d", len(plansHourly))
	}
}

func TestPlanFetchSkipsDisabled(t *testing.T) {
	t.Parallel()
	now := time.Now()
	d1 := now.Add(-1 * time.Hour).Format(time.RFC1123Z)

	ts := mockFeedServer("Disabled Show", []struct{ title, pubDate, url string }{
		{"Ep 1", d1, "http://example.com/dis.mp3"},
	})
	defer ts.Close()

	root := t.TempDir()
	subs := []Subscription{
		{ID: "dis", Title: "Disabled Show", Folder: "Disabled Show", FeedURL: ts.URL, Disabled: true},
	}

	lib := Open(Config{PodcastsDir: root}, nil, nil)
	plans := lib.PlanFetch(subs, FetchOptions{Count: 1}, nil)
	if len(plans) != 0 {
		t.Fatalf("expected 0 plans for disabled subscription, got %d", len(plans))
	}
}

func TestPlanFetchTargetFilter(t *testing.T) {
	t.Parallel()
	now := time.Now()
	d1 := now.Add(-1 * time.Hour).Format(time.RFC1123Z)

	ts1 := mockFeedServer("Alpha Show", []struct{ title, pubDate, url string }{
		{"Ep 1", d1, "http://example.com/alpha.mp3"},
	})
	defer ts1.Close()

	ts2 := mockFeedServer("Beta Show", []struct{ title, pubDate, url string }{
		{"Ep 1", d1, "http://example.com/beta.mp3"},
	})
	defer ts2.Close()

	root := t.TempDir()
	subs := []Subscription{
		{ID: "alpha", Title: "Alpha Show", Folder: "Alpha Show", FeedURL: ts1.URL},
		{ID: "beta", Title: "Beta Show", Folder: "Beta Show", FeedURL: ts2.URL},
	}

	lib := Open(Config{PodcastsDir: root}, nil, nil)
	plans := lib.PlanFetch(subs, FetchOptions{Count: 1, Target: "Beta"}, nil)
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan for target Beta, got %d", len(plans))
	}
	if plans[0].Sub.ID != "beta" {
		t.Errorf("expected beta subscription, got %q", plans[0].Sub.ID)
	}
}

func TestExecuteFetchEnqueuesNewlyDownloaded(t *testing.T) {
	t.Parallel()
	// Audio server that returns fake mp3 bytes
	audioSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("fake audio data"))
	}))
	defer audioSrv.Close()

	root := t.TempDir()
	podDir := filepath.Join(root, "Target Show")
	_ = os.MkdirAll(podDir, 0755)

	store, err := NewSubscriptionStore(filepath.Join(root, "podcasts.json"))
	if err != nil {
		t.Fatal(err)
	}
	sub := Subscription{ID: "tgt", Title: "Target Show", Folder: "Target Show", FeedURL: "http://example.com/rss"}
	_ = store.Add(sub)
	_ = store.Save()

	lib := Open(Config{PodcastsDir: root}, nil, nil)

	ep := backend.FeedEpisode{
		Title:        "New Episode",
		EnclosureURL: audioSrv.URL + "/ep.mp3",
		PubDate:      time.Now().Format(time.RFC1123Z),
	}

	plan := SubscriptionPlan{
		Sub:        sub,
		PodDir:     podDir,
		ToDownload: []backend.FeedEpisode{ep},
	}

	dlOpts := SubscriptionDownloadOptions{
		AlwaysQueue: true,
	}

	res := lib.ExecuteSubscriptionDownloads([]SubscriptionPlan{plan}, store, dlOpts)
	if res.Downloaded != 1 {
		t.Fatalf("expected 1 downloaded episode, got %d (failures: %v)", res.Downloaded, res.Failures)
	}

	// Verify .queue file exists in podDir and contains the downloaded episode!
	queueEntries, err := episode.ReadQueue(podDir)
	if err != nil {
		t.Fatalf("failed to read queue: %v", err)
	}
	if len(queueEntries) != 1 {
		t.Fatalf("expected 1 item in queue, got %d", len(queueEntries))
	}
	if !strings.Contains(queueEntries[0], "New_Episode") {
		t.Errorf("expected queue entry to contain episode name, got %q", queueEntries[0])
	}
}
