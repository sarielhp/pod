package cli

import "testing"

func TestPlayerDaemonAcceptsTheFlagsItsSpawnerPasses(t *testing.T) {
	var action string
	var opts CLIOptions
	err := buildCLIApp(&action, &opts).Execute([]string{"player", "daemon", "/x/ep.mp3", "--title", "Ep 1", "--podcast", "Show"})
	if err != nil {
		t.Fatalf("the daemon subcommand rejected its own spawn arguments: %v", err)
	}
	if opts.PlayerSubcmd != "daemon" || len(opts.Args) != 1 || opts.Args[0] != "/x/ep.mp3" {
		t.Fatalf("parsed subcmd %q args %v", opts.PlayerSubcmd, opts.Args)
	}
	if opts.PlayerTitle != "Ep 1" || opts.Podcast != "Show" {
		t.Fatalf("title %q podcast %q", opts.PlayerTitle, opts.Podcast)
	}
}

func TestPlayerPlayAcceptsQuiet(t *testing.T) {
	var action string
	var opts CLIOptions
	if err := buildCLIApp(&action, &opts).Execute([]string{"player", "play", "e51f41", "-q"}); err != nil {
		t.Fatalf("'player play -q' rejected: %v", err)
	}
	if !opts.Quiet || opts.PlayerSubcmd != "play" || len(opts.Args) != 1 {
		t.Fatalf("parsed quiet=%v subcmd=%q args=%v", opts.Quiet, opts.PlayerSubcmd, opts.Args)
	}
}
