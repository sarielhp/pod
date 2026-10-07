package cli

import (
	"testing"
)

func TestServerCommandFlagParsing(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions

	app := buildCLIApp(&action, &opts)
	if err := app.Execute([]string{"library"}); err != nil {
		t.Fatalf("server execution error: %v", err)
	}
	if action != "library" {
		t.Errorf("server command failed: action=%s", action)
	}
	if opts.EpisodesOnly {
		t.Errorf("server without flags should not be EpisodesOnly")
	}

	action = ""
	opts = CLIOptions{}
	app = buildCLIApp(&action, &opts)
	if err := app.Execute([]string{"library", "-k", "5"}); err != nil {
		t.Fatalf("server -k 5 error: %v", err)
	}
	if !opts.CountGiven || opts.Count != 5 {
		t.Errorf("server -k 5 override failed: CountGiven=%v, Count=%d", opts.CountGiven, opts.Count)
	}

	action = ""
	opts = CLIOptions{}
	app = buildCLIApp(&action, &opts)
	if err := app.Execute([]string{"library", "--episodes-only"}); err != nil {
		t.Fatalf("server --episodes-only execution error: %v", err)
	}
	if action != "library" || !opts.EpisodesOnly {
		t.Errorf("server --episodes-only failed: action=%s, EpisodesOnly=%v", action, opts.EpisodesOnly)
	}

	action = ""
	opts = CLIOptions{}
	app = buildCLIApp(&action, &opts)
	if err := app.Execute([]string{"library", "--episodes-only", "-k", "2"}); err != nil {
		t.Fatalf("server -k 2 error: %v", err)
	}
	if !opts.CountGiven || opts.Count != 2 || !opts.EpisodesOnly {
		t.Errorf("server -k 2 override failed: CountGiven=%v, Count=%d, EpisodesOnly=%v", opts.CountGiven, opts.Count, opts.EpisodesOnly)
	}

	action = ""
	opts = CLIOptions{}
	app = buildCLIApp(&action, &opts)
	if err := app.Execute([]string{"library", "--episodes-only", "-p", "Tech"}); err != nil {
		t.Fatalf("server error: %v", err)
	}
	if action != "library" || !opts.EpisodesOnly || opts.Podcast != "Tech" {
		t.Errorf("server failed: action=%s, podcast=%s", action, opts.Podcast)
	}
}
