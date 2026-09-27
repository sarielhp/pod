package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/types"
)

func testLoadPodcastConfig(podDir string) config.PodcastConfig {
	return config.LoadPodcastConfig(podDir, config.DefaultPodcastConfig(&types.Config{}))
}

func TestFindLatestUncleanedLocalEpisode(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, tmp, "Science Hour", []string{
		"Old Ep",
		"Middle Ep",
		"Newest Ep",
	})

	oldEp := paths[0]
	middleEp := paths[1]
	newestEp := paths[2]

	target, found := findLatestUncleanedLocalEpisode(podDir, "Science Hour", true)
	if !found || target != newestEp {
		t.Fatalf("expected newest uncleaned episode %s, got %s (found=%v)", newestEp, target, found)
	}

	markEpisodeClean(t, newestEp)
	target, found = findLatestUncleanedLocalEpisode(podDir, "Science Hour", true)
	if !found || target != middleEp {
		t.Fatalf("expected middle uncleaned episode %s, got %s (found=%v)", middleEp, target, found)
	}

	markEpisodeClean(t, middleEp)
	markEpisodeClean(t, oldEp)
	target, found = findLatestUncleanedLocalEpisode(podDir, "Science Hour", true)
	if found || target != "" {
		t.Fatalf("expected no uncleaned episode when all clean, got %s (found=%v)", target, found)
	}
}

func TestHandlePodcastRmAdsWorkflow_MultiItemQueueSkip(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, tmp, "History Show", []string{
		"Napoleon Part 1",
		"Napoleon Part 2",
	})

	otherPodDir, otherPaths := createTestPodcastWithEpisodes(t, tmp, "Nature Show", []string{
		"Birds",
	})
	episode.AddToQueue(otherPodDir, filepath.Base(otherPaths[0]))

	podCfg := testLoadPodcastConfig(podDir)
	resolved := &podcast.ResolvedPodcast{
		Dir:        podDir,
		Title:      "History Show",
		ShortID:    podCfg.ID,
		FolderName: "History Show",
		UUID:       podCfg.ID,
		Config:     podCfg,
	}

	opts := types.ProcOptions{
		Quiet: true,
	}
	config := types.Config{PodcastsDir: tmp}

	err := ProcessPodcast(resolved, opts, config)
	if err != nil {
		t.Fatalf("ProcessPodcast failed: %v", err)
	}

	qFile := filepath.Join(podDir, "queue.json")
	data, err := os.ReadFile(qFile)
	if err != nil {
		t.Fatalf("expected queue.json in %s: %v", podDir, err)
	}
	var queued []string
	if err := json.Unmarshal(data, &queued); err != nil {
		t.Fatalf("failed to unmarshal queue: %v", err)
	}
	if len(queued) != 1 || queued[0] != filepath.Base(paths[1]) {
		t.Fatalf("expected %s in queue, got %v", filepath.Base(paths[1]), queued)
	}

	if episode.IsEpisodeClean(paths[1]) {
		t.Fatalf("episode should not have been cleaned immediately when multiple items in queue")
	}
}

func TestHandlePodcastRmAdsWorkflow_DryRun(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, tmp, "Coding Talk", []string{
		"Go 1.26",
	})
	podCfg := testLoadPodcastConfig(podDir)
	resolved := &podcast.ResolvedPodcast{
		Dir:        podDir,
		Title:      "Coding Talk",
		ShortID:    podCfg.ID,
		FolderName: "Coding Talk",
		UUID:       podCfg.ID,
		Config:     podCfg,
	}

	opts := types.ProcOptions{
		Quiet:  true,
		DryRun: true,
	}
	config := types.Config{PodcastsDir: tmp}

	err := ProcessPodcast(resolved, opts, config)
	if err != nil {
		t.Fatalf("dry run workflow failed: %v", err)
	}

	qFile := filepath.Join(podDir, "queue.json")
	if _, err := os.Stat(qFile); err == nil {
		t.Fatalf("expected queue.json not to be created in dry run")
	}
	if episode.IsEpisodeClean(paths[0]) {
		t.Fatalf("episode should not be cleaned in dry run")
	}
}

func TestFindTargetEpisodeFromBackend_FeedCatalog(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, tmp, "Show A", []string{
		"Episode 1",
	})
	markEpisodeClean(t, paths[0])

	b := newMockTestBackend()
	b.podcastMap["item-1"] = &backend.Podcast{
		ID: "item-1",
		Media: backend.PodcastMedia{
			Metadata: backend.PodcastMetadata{Title: "Show A", FeedURL: "https://example.com/feed.xml"},
			Episodes: []backend.Episode{
				{Title: "Episode 1", GUID: "guid-1", EnclosureURL: "https://example.com/ep1.mp3"},
			},
		},
	}
	b.feedEpisodes = []backend.FeedEpisode{
		{Title: "Episode 2", PublishedAt: 1725278400000, GUID: "guid-2", EnclosureURL: "https://example.com/ep2.mp3"},
		{Title: "Episode 1", PublishedAt: 1725192000000, GUID: "guid-1", EnclosureURL: "https://example.com/ep1.mp3"},
	}
	b.downloadFn = func(_ string, _ []backend.FeedEpisode) error {
		return os.WriteFile(filepath.Join(podDir, "Episode_2.mp3"), []byte("new ep 2"), 0644)
	}

	podCfg := testLoadPodcastConfig(podDir)
	resolved := &podcast.ResolvedPodcast{
		Dir:        podDir,
		Title:      "Show A",
		ShortID:    podCfg.ID,
		FolderName: "Show A",
		UUID:       "item-1",
		Config:     podCfg,
	}

	cfg := types.Config{
		PodcastsDir: tmp,
	}
	targetPath, ok := findTargetEpisodeFromBackend(b, resolved, cfg, true)
	if !ok || targetPath == "" {
		t.Fatalf("expected to download and find target episode 2, got %s (ok=%v)", targetPath, ok)
	}
	if filepath.Base(targetPath) != "Episode_2.mp3" && !strings.Contains(targetPath, "Episode") {
		t.Fatalf("unexpected target path: %s", targetPath)
	}
}

func TestFindTargetEpisodeFromBackend_AllClean(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, tmp, "Show B", []string{
		"Episode 1",
	})
	markEpisodeClean(t, paths[0])

	b := newMockTestBackend()
	b.podcastMap["item-b"] = &backend.Podcast{
		ID: "item-b",
		Media: backend.PodcastMedia{
			Metadata: backend.PodcastMetadata{Title: "Show B", FeedURL: "https://example.com/feed.xml"},
			Episodes: []backend.Episode{
				{Title: "Episode 1", GUID: "guid-1", EnclosureURL: "https://example.com/ep1.mp3"},
			},
		},
	}
	b.feedEpisodes = []backend.FeedEpisode{
		{Title: "Episode 1", PublishedAt: 1725192000000, GUID: "guid-1", EnclosureURL: "https://example.com/ep1.mp3"},
	}

	podCfg := testLoadPodcastConfig(podDir)
	resolved := &podcast.ResolvedPodcast{
		Dir:        podDir,
		Title:      "Show B",
		ShortID:    podCfg.ID,
		FolderName: "Show B",
		UUID:       "item-b",
		Config:     podCfg,
	}

	cfg := types.Config{
		PodcastsDir: tmp,
	}
	targetPath, handled := findTargetEpisodeFromBackend(b, resolved, cfg, true)
	if !handled || targetPath != "" {
		t.Fatalf("expected handled=true and targetPath='' when all episodes clean, got %s (handled=%v)", targetPath, handled)
	}
}

func TestProcessSingleQueuedTarget_LocalCompletion(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, tmp, "Clean Show", []string{
		"Episode Test",
	})
	targetAudio := paths[0]
	epFilename := filepath.Base(targetAudio)
	episode.AddToQueue(podDir, epFilename)

	markEpisodeClean(t, targetAudio)

	optsLocal := types.ProcOptions{
		Quiet: true,
	}
	err := ProcessQueuedTarget(podDir, targetAudio, optsLocal, types.Config{PodcastsDir: tmp})
	if err != nil {
		t.Fatalf("ProcessQueuedTarget failed: %v", err)
	}

	qFile := filepath.Join(podDir, "queue.json")
	data, _ := os.ReadFile(qFile)
	var entries []string
	_ = json.Unmarshal(data, &entries)
	for _, e := range entries {
		if strings.EqualFold(e, epFilename) {
			t.Fatalf("expected %s to be removed from queue.json, but was still present", epFilename)
		}
	}
}

func TestHandlePodcastRmAdsWorkflow_OfflineBackendFallback(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, tmp, "Offline Show", []string{
		"Episode 1",
		"Episode 2",
	})
	markEpisodeClean(t, paths[0])

	podCfg := testLoadPodcastConfig(podDir)
	resolved := &podcast.ResolvedPodcast{
		Dir:        podDir,
		Title:      "Offline Show",
		ShortID:    podCfg.ID,
		FolderName: "Offline Show",
		UUID:       podCfg.ID,
		Config:     podCfg,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := types.Config{
		PodcastsDir: tmp,
		BackendConfig: types.BackendConfig{
			BackendType: "podfetch",
			PodfetchURL: srv.URL,
		},
	}

	targetPath, err := resolveTargetEpisodeForRmAds(resolved, types.ProcOptions{
		Quiet: true,
	}, cfg)
	if err != nil {
		t.Fatalf("unexpected error during offline fallback: %v", err)
	}
	if targetPath != paths[1] {
		t.Fatalf("expected offline fallback to select %s, got %s", paths[1], targetPath)
	}
}

func TestHandlePodcastRmAdsWorkflow_QueueSingleAndRemove(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, tmp, "Solo Show", []string{
		"Ep 1",
	})
	markEpisodeClean(t, paths[0])

	podCfg := testLoadPodcastConfig(podDir)
	resolved := &podcast.ResolvedPodcast{
		Dir:        podDir,
		Title:      "Solo Show",
		ShortID:    podCfg.ID,
		FolderName: "Solo Show",
		UUID:       podCfg.ID,
		Config:     podCfg,
	}

	opts := types.ProcOptions{
		Quiet: true,
	}
	config := types.Config{PodcastsDir: tmp}

	episode.AddToQueue(resolved.Dir, filepath.Base(paths[0]))

	err := ProcessQueuedTarget(resolved.Dir, paths[0], opts, config)
	if err != nil {
		t.Fatalf("ProcessQueuedTarget failed: %v", err)
	}

	qFile := filepath.Join(resolved.Dir, "queue.json")
	data, err := os.ReadFile(qFile)
	if err == nil {
		var entries []string
		_ = json.Unmarshal(data, &entries)
		for _, e := range entries {
			if strings.EqualFold(e, filepath.Base(paths[0])) {
				t.Fatalf("expected episode to be removed from queue file after processing")
			}
		}
	}
}

func TestCountAllQueuedEpisodes(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	p1, _ := createTestPodcastWithEpisodes(t, tmp, "Show 1", []string{"E1"})
	p2, _ := createTestPodcastWithEpisodes(t, tmp, "Show 2", []string{"E2"})

	if count := countAllQueuedEpisodes(tmp); count != 0 {
		t.Fatalf("expected 0 queued, got %d", count)
	}

	episode.AddToQueue(p1, "E1.mp3")
	if count := countAllQueuedEpisodes(tmp); count != 1 {
		t.Fatalf("expected 1 queued, got %d", count)
	}

	episode.AddToQueue(p2, "E2.mp3")
	if count := countAllQueuedEpisodes(tmp); count != 2 {
		t.Fatalf("expected 2 queued, got %d", count)
	}
}

func TestResolveMatchingEpisodeAudioFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir := filepath.Join(tmp, "My Show")
	epDir := filepath.Join(podDir, "Episode 1 Subfolder")
	if err := os.MkdirAll(epDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	mp3Path := filepath.Join(epDir, "podcast.mp3")
	if err := os.WriteFile(mp3Path, []byte("test audio"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	cases := []string{
		mp3Path,
		"My Show/Episode 1 Subfolder/podcast.mp3",
		"podcasts/My Show/Episode 1 Subfolder/podcast.mp3",
		"/podcasts/My Show/Episode 1 Subfolder/podcast.mp3",
		"Episode 1 Subfolder/podcast.mp3",
	}

	for _, c := range cases {
		ep := backend.Episode{
			AudioFile: &backend.PodcastAudioFile{
				Metadata: &backend.AudioFileMetadata{
					Path:     c,
					Filename: "podcast.mp3",
				},
			},
		}
		got, ok := resolveMatchingEpisodeAudioFile(podDir, ep)
		if !ok || got != mp3Path {
			t.Errorf("for path %q, expected (%s, true), got (%s, %v)", c, mp3Path, got, ok)
		}
	}
}

func TestFindLocalPathForFeedEpisode_SubfolderFuzzy(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir := filepath.Join(tmp, "Haaretz Weekly")
	subDir := filepath.Join(podDir, "-קיבלתי את המידע מיד אחרי הטבח. לקח לי שנה וחצי לוודא שנתניהו הוזהר לפני 7.10- - פרק 670")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	mp3Path := filepath.Join(subDir, "podcast.mp3")
	if err := os.WriteFile(mp3Path, []byte("audio"), 0644); err != nil {
		t.Fatalf("write mp3 failed: %v", err)
	}

	fe := backend.FeedEpisode{
		Title: "\"קיבלתי את המידע מיד אחרי הטבח. לקח לי שנה וחצי לוודא שנתניהו הוזהר לפני 7.10\" | פרק 670",
	}

	got, ok := findLocalPathForFeedEpisode(podDir, fe, nil)
	if !ok || got != mp3Path {
		t.Fatalf("expected (%s, true), got (%s, %v)", mp3Path, got, ok)
	}
}

func TestFindTargetEpisodeFromBackend_SubfolderUncleaned(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	podDir := filepath.Join(tmp, "Haaretz Show")
	subDir := filepath.Join(podDir, "-קיבלתי את המידע מיד אחרי הטבח- - פרק 670")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	mp3Path := filepath.Join(subDir, "podcast.mp3")
	if err := os.WriteFile(mp3Path, []byte("audio"), 0644); err != nil {
		t.Fatalf("write mp3 failed: %v", err)
	}
	statPath := episode.StatusPathFor(mp3Path)
	_ = episode.SaveEpisodeStatus(statPath, &types.EpisodeStatusFile{
		MediaFile: "podcast.mp3",
		Status:    types.StateDownloaded,
		Original:  types.EpisodeAudioMeta{DurationSec: 100},
	})

	b := newMockTestBackend()
	b.podcastMap["item-h"] = &backend.Podcast{
		ID: "item-h",
		Media: backend.PodcastMedia{
			Metadata: backend.PodcastMetadata{Title: "Haaretz Show", FeedURL: "https://example.com/feed.xml"},
			Episodes: []backend.Episode{
				{
					Title:        "\"קיבלתי את המידע מיד אחרי הטבח\" | פרק 670",
					GUID:         "guid-670",
					EnclosureURL: "https://example.com/audio.mp3",
					AudioFile: &backend.PodcastAudioFile{
						Metadata: &backend.AudioFileMetadata{
							Path:     "Haaretz Show/-קיבלתי את המידע מיד אחרי הטבח- - פרק 670/podcast.mp3",
							Filename: "podcast.mp3",
						},
					},
				},
			},
		},
	}
	b.feedEpisodes = []backend.FeedEpisode{
		{
			Title:        "\"קיבלתי את המידע מיד אחרי הטבח\" | פרק 670",
			GUID:         "guid-670",
			EnclosureURL: "https://example.com/audio.mp3",
			PublishedAt:  1725364800000,
		},
	}

	resolved := &podcast.ResolvedPodcast{
		Dir:        podDir,
		Title:      "Haaretz Show",
		ShortID:    "h123",
		FolderName: "Haaretz Show",
		UUID:       "item-h",
	}

	cfg := types.Config{
		PodcastsDir: tmp,
	}
	targetPath, ok := findTargetEpisodeFromBackend(b, resolved, cfg, true)
	if !ok || targetPath != mp3Path {
		t.Fatalf("expected targetPath=%s (ok=true), got %s (ok=%v)", mp3Path, targetPath, ok)
	}
}
