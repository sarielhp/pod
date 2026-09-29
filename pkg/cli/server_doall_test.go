package cli

import (
	"reflect"
	"testing"
)

func TestDoAllRunsStagesInDependencyOrder(t *testing.T) {
	var got []string
	for _, s := range doAllSteps() {
		got = append(got, s.name)
	}
	want := []string{
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
