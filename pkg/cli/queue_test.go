package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

func TestQueueListEmpty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	cfg := Config{PodcastsDir: tempDir}

	cli := CLIOptions{QueueSubcmd: "list"}
	cli.Out = &buf
	cli.Err = &buf
	err := runQueueCommand(cfg, cli)

	if err != nil {
		t.Fatalf("runQueueCommand failed: %v", err)
	}

	outBytes := buf.Bytes()
	if !strings.Contains(string(outBytes), "empty") {
		t.Errorf("expected empty queue message, got: %s", string(outBytes))
	}
}

func TestQueueLsShowsQueuedEpisodes(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	root := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, root, "Show", []string{"Episode One"})
	episode.AddToQueue(podDir, filepath.Base(paths[0]))
	for _, command := range []string{"list", "ls"} {
		var action string
		var opts CLIOptions
		app := buildCLIApp(&action, &opts)
		if err := app.Execute([]string{"queue", command, "--json"}); err != nil {
			t.Fatal(err)
		}
		buf.Reset()
		opts.Out = &buf
		opts.Err = &buf
		if err := runQueueCommand(Config{PodcastsDir: root}, opts); err != nil {
			t.Fatalf("run: %v", err)
		}
		data := buf.Bytes()
		var items []queueEpisodeItem
		if err := json.Unmarshal(data, &items); err != nil {
			t.Fatal(err)
		}
		if action != "queue" || len(items) != 1 || items[0].AudioPath != paths[0] {
			t.Fatalf("%s returned %s: %s", command, action, data)
		}
	}
}

func testQueueAddAndList(t *testing.T, cfg Config, podDir, ep1ID string) {
	var buf bytes.Buffer
	cliAdd := CLIOptions{QueueSubcmd: "add", Args: []string{ep1ID}}
	cliAdd.Out = &buf
	cliAdd.Err = &buf
	if err := runQueueCommand(cfg, cliAdd); err != nil {
		t.Fatalf("runQueueCommand add failed: %v", err)
	}

	qFile := filepath.Join(podDir, "queue.json")
	data, err := os.ReadFile(qFile)
	if err != nil {
		t.Fatalf("expected queue.json to exist: %v", err)
	}
	var entries []string
	_ = json.Unmarshal(data, &entries)
	if len(entries) != 1 || entries[0] != "ep1.mp3" {
		t.Errorf("expected [ep1.mp3] in queue, got: %v", entries)
	}

	buf.Reset()
	cliListJSON := CLIOptions{QueueSubcmd: "list", JSON: true}
	cliListJSON.Out = &buf
	cliListJSON.Err = &buf
	_ = runQueueCommand(cfg, cliListJSON)

	outBytes := buf.Bytes()
	var listItems []queueEpisodeItem
	if err := json.Unmarshal(outBytes, &listItems); err != nil || len(listItems) != 1 {
		t.Fatalf("failed to parse queue json: %v, got %s", err, string(outBytes))
	}
	if listItems[0].EpisodeID != ep1ID {
		t.Errorf("expected episode %s in list, got %s", ep1ID, listItems[0].EpisodeID)
	}
}

func testQueueRemoveAndClear(t *testing.T, cfg Config, podDir, podID, ep1ID string) {
	var buf bytes.Buffer
	qFile := filepath.Join(podDir, "queue.json")
	cliRemove := CLIOptions{QueueSubcmd: "remove", Args: []string{ep1ID}}
	cliRemove.Out = &buf
	cliRemove.Err = &buf
	if err := runQueueCommand(cfg, cliRemove); err != nil {
		t.Fatalf("runQueueCommand remove failed: %v", err)
	}
	data, _ := os.ReadFile(qFile)
	var entries []string
	_ = json.Unmarshal(data, &entries)
	if len(entries) != 0 {
		t.Errorf("expected empty queue after removal, got: %v", entries)
	}

	cliAddPod := CLIOptions{QueueSubcmd: "add", Args: []string{podID}}
	cliAddPod.Out = &buf
	cliAddPod.Err = &buf
	if err := runQueueCommand(cfg, cliAddPod); err != nil {
		t.Fatalf("runQueueCommand add podcast failed: %v", err)
	}
	data, _ = os.ReadFile(qFile)
	_ = json.Unmarshal(data, &entries)
	if len(entries) != 2 {
		t.Errorf("expected 2 episodes in queue after adding podcast, got: %v", entries)
	}

	cliClear := CLIOptions{QueueSubcmd: "clear", Args: []string{podID}}
	cliClear.Out = &buf
	cliClear.Err = &buf
	if err := runQueueCommand(cfg, cliClear); err != nil {
		t.Fatalf("runQueueCommand clear failed: %v", err)
	}
	data, _ = os.ReadFile(qFile)
	_ = json.Unmarshal(data, &entries)
	if len(entries) != 0 {
		t.Errorf("expected empty queue after clear, got: %v", entries)
	}
}

func TestQueueAddRemoveAndClear(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "Show_Q")
	_ = os.MkdirAll(podDir, 0755)

	ep1 := filepath.Join(podDir, "ep1.mp3")
	ep2 := filepath.Join(podDir, "ep2.mp3")
	_ = os.WriteFile(ep1, []byte("audio1"), 0644)
	_ = os.WriteFile(ep2, []byte("audio2"), 0644)

	podID := podcast.GetOrSetPodcastShortID(podDir, "Show_Q")
	ep1ID := podcast.GetOrSetEpisodeShortID(podDir, podID, ep1)
	_ = podcast.GetOrSetEpisodeShortID(podDir, podID, ep2)

	cfg := Config{PodcastsDir: tempDir}
	testQueueAddAndList(t, cfg, podDir, ep1ID)
	testQueueRemoveAndClear(t, cfg, podDir, podID, ep1ID)
}

func TestUpdateQueue_ConcurrentTransactions(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	var wg syncWG
	for i := 0; i < 10; i++ {
		fn := fmt.Sprintf("ep%d.mp3", i)
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			episode.AddToQueue(tempDir, name)
		}(fn)
	}
	wg.Wait()

	qFile := filepath.Join(tempDir, "queue.json")
	data, err := os.ReadFile(qFile)
	if err != nil {
		t.Fatalf("expected queue.json: %v", err)
	}
	var entries []string
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(entries) != 10 {
		t.Fatalf("expected exactly 10 episodes in queue, got %d: %v", len(entries), entries)
	}

	for i := 0; i < 5; i++ {
		fn := fmt.Sprintf("ep%d.mp3", i)
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			episode.RemoveFromQueue(tempDir, name)
		}(fn)
	}
	wg.Wait()

	data, _ = os.ReadFile(qFile)
	_ = json.Unmarshal(data, &entries)
	if len(entries) != 5 {
		t.Fatalf("expected exactly 5 episodes in queue after concurrent remove, got %d", len(entries))
	}
}

func TestPrintQueueTableHebrew(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	items := []queueEpisodeItem{
		{
			PodcastID: "pod1",
			EpisodeID: "ep01",
			Title:     "פרק מיוחד בעברית",
			AudioPath: "/podcasts/heb/ep01.mp3",
			Filename:  "פרק מיוחד בעברית.mp3",
		},
	}

	printQueueTable(&buf, items)

	outBytes := buf.Bytes()
	out := string(outBytes)

	expected := util.DisplayName("פרק מיוחד בעברית")
	if !strings.Contains(out, expected) {
		t.Errorf("expected queue table to contain %q, got: %s", expected, out)
	}
}

func TestQueueRun_Empty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	cfg := Config{PodcastsDir: tempDir}
	cli := CLIOptions{
		ProcOptions: ProcOptions{
			Quiet: true,
		},
		QueueSubcmd: "run",
	}
	cli.Out = &buf
	cli.Err = &buf
	if err := runQueueCommand(cfg, cli); err != nil {
		t.Fatalf("expected nil error on empty queue run, got: %v", err)
	}
}

func TestQueueRun_DryRun(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, tempDir, "DryShow", []string{"Ep1", "Ep2"})
	episode.AddToQueue(podDir, filepath.Base(paths[0]))
	episode.AddToQueue(podDir, filepath.Base(paths[1]))

	cfg := Config{PodcastsDir: tempDir}
	cli := CLIOptions{
		ProcOptions: ProcOptions{
			DryRun: true,
			Quiet:  true,
		},
		QueueSubcmd: "run",
	}
	cli.Out = &buf
	cli.Err = &buf
	if err := runQueueCommand(cfg, cli); err != nil {
		t.Fatalf("runQueueCommand dry-run failed: %v", err)
	}

	qFile := filepath.Join(podDir, "queue.json")
	data, _ := os.ReadFile(qFile)
	var entries []string
	_ = json.Unmarshal(data, &entries)
	if len(entries) != 2 {
		t.Fatalf("expected 2 episodes to remain in queue after dry run, got %d", len(entries))
	}
}

func TestQueueRun_CleansAndDequeues(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	podDir, paths := createTestPodcastWithEpisodes(t, tempDir, "QueueShow", []string{"Ep1", "Ep2"})
	episode.AddToQueue(podDir, filepath.Base(paths[0]))
	episode.AddToQueue(podDir, filepath.Base(paths[1]))

	markEpisodeClean(t, paths[0])
	markEpisodeClean(t, paths[1])

	cfg := Config{PodcastsDir: tempDir}
	cli := CLIOptions{
		ProcOptions: ProcOptions{
			Quiet: true,
		},
		QueueSubcmd: "run",
	}
	cli.Out = &buf
	cli.Err = &buf
	if err := runQueueCommand(cfg, cli); err != nil {
		t.Fatalf("runQueueCommand run failed: %v", err)
	}

	qFile := filepath.Join(podDir, "queue.json")
	data, _ := os.ReadFile(qFile)
	var entries []string
	_ = json.Unmarshal(data, &entries)
	if len(entries) != 0 {
		t.Fatalf("expected 0 episodes in queue after processing clean episodes, got %d", len(entries))
	}
}

func TestQueueRun_SpecificTarget(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	pod1Dir, paths1 := createTestPodcastWithEpisodes(t, tempDir, "PodA", []string{"EpA"})
	pod2Dir, paths2 := createTestPodcastWithEpisodes(t, tempDir, "PodB", []string{"EpB"})

	episode.AddToQueue(pod1Dir, filepath.Base(paths1[0]))
	episode.AddToQueue(pod2Dir, filepath.Base(paths2[0]))

	markEpisodeClean(t, paths1[0])
	markEpisodeClean(t, paths2[0])

	pod1Cfg := config.LoadPodcastConfig(pod1Dir, config.PodcastConfig{})
	cfg := Config{PodcastsDir: tempDir}
	cli := CLIOptions{
		ProcOptions: ProcOptions{
			Quiet: true,
		},
		QueueSubcmd: "run",
		Args:        []string{pod1Cfg.ID},
	}
	cli.Out = &buf
	cli.Err = &buf
	if err := runQueueCommand(cfg, cli); err != nil {
		t.Fatalf("runQueueCommand target failed: %v", err)
	}

	q1, _ := os.ReadFile(filepath.Join(pod1Dir, "queue.json"))
	var entries1 []string
	_ = json.Unmarshal(q1, &entries1)
	if len(entries1) != 0 {
		t.Fatalf("expected PodA queue to be empty, got %v", entries1)
	}

	q2, _ := os.ReadFile(filepath.Join(pod2Dir, "queue.json"))
	var entries2 []string
	_ = json.Unmarshal(q2, &entries2)
	if len(entries2) != 1 {
		t.Fatalf("expected PodB queue to still have 1 entry, got %v", entries2)
	}
}

func TestQueueRun_MissingFileRetained(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	podDir, _ := createTestPodcastWithEpisodes(t, tempDir, "MissingShow", []string{})
	episode.AddToQueue(podDir, "nonexistent.mp3")

	cfg := Config{PodcastsDir: tempDir}
	cli := CLIOptions{
		ProcOptions: ProcOptions{
			Quiet: true,
		},
		QueueSubcmd: "run",
	}
	cli.Out = &buf
	cli.Err = &buf
	if err := runQueueCommand(cfg, cli); err == nil {
		t.Fatal("missing file should report an error")
	}

	qFile := filepath.Join(podDir, "queue.json")
	data, _ := os.ReadFile(qFile)
	var entries []string
	_ = json.Unmarshal(data, &entries)
	if len(entries) != 1 {
		t.Fatalf("expected missing file to remain in queue, got %v", entries)
	}
}

func TestQueueAddAllPreservesNestedEpisodePaths(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "NestedShow")
	audioPath := filepath.Join(podDir, "2026", "ep1.mp3")
	if err := os.MkdirAll(filepath.Dir(audioPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{PodcastsDir: tempDir}
	if err := runQueueCommand(cfg, CLIOptions{QueueSubcmd: "add", Args: []string{"all"}}); err != nil {
		t.Fatalf("queue add all: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(podDir, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []string
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0] != filepath.Join("2026", "ep1.mp3") {
		t.Fatalf("queue entries = %v, want nested relative path", entries)
	}

	markEpisodeClean(t, audioPath)
	cli := CLIOptions{QueueSubcmd: "run", ProcOptions: ProcOptions{Quiet: true}}
	cli.Out = &buf
	cli.Err = &buf
	if err := runQueueCommand(cfg, cli); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(podDir, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("queue entries after run = %v, want empty", entries)
	}
}

func TestQueueRunResolvesLegacyNestedFilename(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tempDir := t.TempDir()
	podDir := filepath.Join(tempDir, "LegacyShow")
	audioPath := filepath.Join(podDir, "2026", "ep1.mp3")
	if err := os.MkdirAll(filepath.Dir(audioPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(audioPath, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}
	episode.AddToQueue(podDir, filepath.Base(audioPath))
	markEpisodeClean(t, audioPath)

	cfg := Config{PodcastsDir: tempDir}
	cli := CLIOptions{QueueSubcmd: "run", ProcOptions: ProcOptions{Quiet: true}}
	cli.Out = &buf
	cli.Err = &buf
	if err := runQueueCommand(cfg, cli); err != nil {
		t.Fatalf("queue run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(podDir, "queue.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []string
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("legacy queue entries after run = %v, want empty", entries)
	}
}

func TestQueueRun_CLIHelpParsing(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	if err := app.Execute([]string{"queue", "run", "pod-1", "--dry-run", "--quiet"}); err != nil {
		t.Fatalf("expected queue run to parse: %v", err)
	}
	if action != "queue" || opts.QueueSubcmd != "run" {
		t.Fatalf("expected action=queue, subcmd=run, got action=%s, subcmd=%s", action, opts.QueueSubcmd)
	}
	if !opts.DryRun || !opts.Quiet {
		t.Fatalf("expected dry-run and quiet to be true")
	}
	if len(opts.Args) != 1 || opts.Args[0] != "pod-1" {
		t.Fatalf("expected args ['pod-1'], got %v", opts.Args)
	}
}
