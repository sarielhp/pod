package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"pod/pkg/util"
)

func TestDoAllRunsStagesInDependencyOrder(t *testing.T) {
	var got []string
	for _, s := range doAllSteps() {
		got = append(got, s.name)
	}
	want := []string{
		"prune older episodes of non-favorite podcasts",
		"fetch new episodes",
		"transcribe episodes missing a transcript",
		"analyze boilerplate",
		"remove ads from queued episodes",
		"cut boilerplate from finished episodes",
		"republish feeds",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stages = %v, want %v", got, want)
	}
}

func TestDoAllScopePassesThePodcastToEveryStage(t *testing.T) {
	var cli CLIOptions
	cli.Args = []string{"stray"}
	if got := doAllScope(cli).Args; got != nil {
		t.Fatalf("no podcast: args = %v, want none", got)
	}
	cli.Podcast = "Fresh Air"
	if got := doAllScope(cli).Args; !reflect.DeepEqual(got, []string{"Fresh Air"}) {
		t.Fatalf("args = %v", got)
	}
}

func TestDoAllPruneStepKeepsFiveLatestAndSparesFavorites(t *testing.T) {
	root := t.TempDir()
	plain := makePruneShow(t, root, "Plain", 7, false)
	loved := makePruneShow(t, root, "Loved", 7, true)
	for _, dir := range []string{plain, loved} {
		for i := 0; i < 7; i++ {
			precut := filepath.Join(dir, fmt.Sprintf("ep%d.mp3.precut", i))
			if err := os.WriteFile(precut, []byte("precut-audio"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	cfg := Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}
	var out bytes.Buffer
	cli := CLIOptions{Out: &out, Err: &out}

	steps := doAllSteps()
	pruneStep := steps[0]
	if pruneStep.name != "prune older episodes of non-favorite podcasts" {
		t.Fatalf("first step is %q", pruneStep.name)
	}

	cli.DryRun = true
	if err := pruneStep.run(cfg, cli); err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if !util.FileExists(filepath.Join(plain, "ep6.mp3")) {
		t.Fatalf("dry run deleted file")
	}

	cli.DryRun = false
	if err := pruneStep.run(cfg, cli); err != nil {
		t.Fatalf("real run failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		if !util.FileExists(filepath.Join(plain, fmt.Sprintf("ep%d.mp3", i))) {
			t.Errorf("ep%d.mp3 should be kept", i)
		}
	}
	for i := 5; i < 7; i++ {
		if util.FileExists(filepath.Join(plain, fmt.Sprintf("ep%d.mp3", i))) {
			t.Errorf("ep%d.mp3 should be deleted", i)
		}
		if util.FileExists(filepath.Join(plain, fmt.Sprintf("ep%d.precut", i))) || util.FileExists(filepath.Join(plain, fmt.Sprintf("ep%d.mp3.precut", i))) {
			t.Errorf("ep%d precut should be deleted", i)
		}
		if !util.FileExists(filepath.Join(plain, fmt.Sprintf("ep%d.transcript.json", i))) {
			t.Errorf("ep%d.transcript.json should be kept", i)
		}
	}

	for i := 0; i < 7; i++ {
		if !util.FileExists(filepath.Join(loved, fmt.Sprintf("ep%d.mp3", i))) {
			t.Errorf("loved ep%d.mp3 should be kept", i)
		}
		if !util.FileExists(filepath.Join(loved, fmt.Sprintf("ep%d.mp3.precut", i))) {
			t.Errorf("loved ep%d.mp3.precut should be kept", i)
		}
	}
}

func TestDoAllPruneStepScopedToSinglePodcast(t *testing.T) {
	root := t.TempDir()
	plain1 := makePruneShow(t, root, "Plain1", 7, false)
	plain2 := makePruneShow(t, root, "Plain2", 7, false)

	cfg := Config{PodcastsDir: root, SubscriptionsFile: filepath.Join(root, "s.json")}
	var out bytes.Buffer
	cli := CLIOptions{Out: &out, Err: &out, Podcast: "Plain1"}

	steps := doAllSteps()
	pruneStep := steps[0]
	if err := pruneStep.run(cfg, doAllScope(cli)); err != nil {
		t.Fatalf("scoped run failed: %v", err)
	}

	if util.FileExists(filepath.Join(plain1, "ep6.mp3")) {
		t.Errorf("Plain1 ep6.mp3 should have been deleted")
	}
	if !util.FileExists(filepath.Join(plain2, "ep6.mp3")) {
		t.Errorf("Plain2 ep6.mp3 should have been kept")
	}
}
