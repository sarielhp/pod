package cli

import (
	"path/filepath"
	"pod/pkg/podcast"
	"reflect"
	"testing"

	"pod/pkg/episode"
)

func TestQueueLatestDefaultCount(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	titles := []string{"Ep1", "Ep2", "Ep3", "Ep4", "Ep5", "Ep6", "Ep7", "Ep8", "Ep9", "Ep10", "Ep11", "Ep12"}
	dir, paths := createTestPodcastWithEpisodes(t, root, "Show", titles)

	opts := CLIOptions{
		ProcOptions: ProcOptions{
			Quiet: true,
		},
	}

	if err := runQueueLatest(root, 10, "", opts); err != nil {
		t.Fatal(err)
	}

	queue, err := episode.ReadQueue(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(queue) != 10 {
		t.Fatalf("expected 10 queued episodes, got %d: %v", len(queue), queue)
	}

	expected := []string{
		filepath.Base(paths[11]),
		filepath.Base(paths[10]),
		filepath.Base(paths[9]),
		filepath.Base(paths[8]),
		filepath.Base(paths[7]),
		filepath.Base(paths[6]),
		filepath.Base(paths[5]),
		filepath.Base(paths[4]),
		filepath.Base(paths[3]),
		filepath.Base(paths[2]),
	}
	if !reflect.DeepEqual(queue, expected) {
		t.Fatalf("queue mismatch: got %v, want %v", queue, expected)
	}
}

func TestQueueLatestExcludesClean(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	titles := []string{"Old", "Middle", "Newest"}
	dir, paths := createTestPodcastWithEpisodes(t, root, "Show", titles)

	markEpisodeClean(t, paths[2])

	opts := CLIOptions{
		ProcOptions: ProcOptions{
			Quiet: true,
		},
	}

	if err := runQueueLatest(root, 2, "", opts); err != nil {
		t.Fatal(err)
	}

	queue, err := episode.ReadQueue(dir)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		filepath.Base(paths[1]),
		filepath.Base(paths[0]),
	}
	if !reflect.DeepEqual(queue, expected) {
		t.Fatalf("queue mismatch: got %v, want %v", queue, expected)
	}
}

func TestQueueLatestTargetPodcast(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dirA, pathsA := createTestPodcastWithEpisodes(t, root, "ShowA", []string{"A1", "A2"})
	dirB, _ := createTestPodcastWithEpisodes(t, root, "ShowB", []string{"B1", "B2"})

	opts := CLIOptions{
		ProcOptions: ProcOptions{
			Quiet: true,
		},
	}

	if err := runQueueLatest(root, 5, "showa", opts); err != nil {
		t.Fatal(err)
	}

	queueA, err := episode.ReadQueue(dirA)
	if err != nil {
		t.Fatal(err)
	}
	if len(queueA) != 2 {
		t.Fatalf("expected 2 items in ShowA queue, got %d: %v", len(queueA), queueA)
	}
	expectedA := []string{filepath.Base(pathsA[1]), filepath.Base(pathsA[0])}
	if !reflect.DeepEqual(queueA, expectedA) {
		t.Fatalf("ShowA mismatch: got %v, want %v", queueA, expectedA)
	}

	queueB, _ := episode.ReadQueue(dirB)
	if len(queueB) != 0 {
		t.Fatalf("expected ShowB queue to be empty, got %v", queueB)
	}
}

func TestQueueLatestDryRun(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir, _ := createTestPodcastWithEpisodes(t, root, "Show", []string{"Ep1", "Ep2"})

	opts := CLIOptions{
		ProcOptions: ProcOptions{
			DryRun: true,
			Quiet:  true,
		},
	}

	if err := runQueueLatest(root, 5, "", opts); err != nil {
		t.Fatal(err)
	}

	queue, _ := episode.ReadQueue(dir)
	if len(queue) != 0 {
		t.Fatalf("dry run should not modify queue, got: %v", queue)
	}
}

func TestQueueLatestParsing(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	if err := app.Execute([]string{"queue", "latest", "5", "--dry-run", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	if action != "queue" || opts.QueueSubcmd != "latest" || !opts.DryRun || !opts.Quiet {
		t.Fatalf("unexpected parsed command: %s %+v", action, opts)
	}

	limit, target, err := parseQueueLatestArgs(opts.Args)
	if err != nil || limit != 5 || target != "" {
		t.Fatalf("parseQueueLatestArgs mismatch: limit=%d, target=%q, err=%v", limit, target, err)
	}

	limit, target, err = parseQueueLatestArgs([]string{"3", "mypod"})
	if err != nil || limit != 3 || target != "mypod" {
		t.Fatalf("parseQueueLatestArgs mismatch: limit=%d, target=%q, err=%v", limit, target, err)
	}

	limit, target, err = parseQueueLatestArgs([]string{"mypod", "3"})
	if err != nil || limit != 3 || target != "mypod" {
		t.Fatalf("parseQueueLatestArgs reversed mismatch: limit=%d, target=%q, err=%v", limit, target, err)
	}

	limit, target, err = parseQueueLatestArgs([]string{})
	if err != nil || limit != 10 || target != "" {
		t.Fatalf("parseQueueLatestArgs empty mismatch: limit=%d, target=%q, err=%v", limit, target, err)
	}

	limit, target, err = parseQueueLatestArgs([]string{}, 20)
	if err != nil || limit != 20 || target != "" {
		t.Fatalf("parseQueueLatestArgs default mismatch: limit=%d, target=%q, err=%v", limit, target, err)
	}

	if _, _, err := parseQueueLatestArgs([]string{"0"}); err == nil {
		t.Fatal("expected error for limit 0")
	}

	if _, _, err := parseQueueLatestArgs([]string{"-2"}); err == nil {
		t.Fatal("expected error for negative limit")
	}

	if _, _, err := parseQueueLatestArgs([]string{"pod1", "pod2"}); err == nil {
		t.Fatal("expected error for multiple target names")
	}
}

func TestQueueRoutingLatest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir, paths := createTestPodcastWithEpisodes(t, root, "Show", []string{"E1", "E2", "E3"})

	cfg := Config{PodcastsDir: root}

	// Test abs queue latest 2
	err := runQueueCommand(cfg, CLIOptions{
		QueueSubcmd: "latest",
		Args:        []string{"2"},
		ProcOptions: ProcOptions{Quiet: true},
	})
	if err != nil {
		t.Fatalf("runQueueCommand latest 2 failed: %v", err)
	}
	q, _ := episode.ReadQueue(dir)
	if len(q) != 2 {
		t.Fatalf("expected 2 queued items, got %d", len(q))
	}

	// Test abs queue latest without count (defaults to 10, queues remaining available)
	err = runQueueCommand(cfg, CLIOptions{
		QueueSubcmd: "latest",
		ProcOptions: ProcOptions{Quiet: true},
	})
	if err != nil {
		t.Fatalf("runQueueCommand latest default failed: %v", err)
	}
	q, _ = episode.ReadQueue(dir)
	if len(q) != 3 {
		t.Fatalf("expected 3 queued items, got %d", len(q))
	}

	// Test abs queue add latest 1
	_ = podcast.ClearPodcastQueue(dir)
	err = runQueueCommand(cfg, CLIOptions{
		QueueSubcmd: "add",
		Args:        []string{"latest", "1"},
		ProcOptions: ProcOptions{Quiet: true},
	})
	if err != nil {
		t.Fatalf("runQueueCommand add latest 1 failed: %v", err)
	}
	q, _ = episode.ReadQueue(dir)
	if len(q) != 1 || q[0] != filepath.Base(paths[2]) {
		t.Fatalf("expected 1 item (newest), got %v", q)
	}
}
