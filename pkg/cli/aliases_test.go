package cli

import (
	"strings"
	"testing"

	"github.com/sarielhp/clihelp"
)

func commandPaths(t *testing.T, app *clihelp.App) map[string]bool {
	t.Helper()
	paths := map[string]bool{}
	err := app.Walk(func(path []string, cmd *clihelp.Command) error {
		joined := strings.Join(path, " ")
		paths[joined] = true
		if len(cmd.Aliases) > 0 {
			t.Errorf("command %q declares aliases %v; AGENTS.md rule 11 allows one canonical name per command", joined, cmd.Aliases)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return paths
}

func TestNoCommandHasAnAlias(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	paths := commandPaths(t, app)
	for _, removed := range []string{"ui", "queue ls", "server feeds update"} {
		if paths[removed] {
			t.Errorf("%q is registered as a command, duplicating a canonical one", removed)
		}
	}
}

func TestRemovedAliasesAreNotAccepted(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	if err := app.Execute([]string{"ui"}); err == nil {
		t.Errorf("expected 'pod ui' to be rejected; the command is 'pod tui'")
	}

	action, opts = "", CLIOptions{}
	if err := app.Execute([]string{"tui"}); err != nil || action != "tui" {
		t.Errorf("expected 'pod tui' to run the TUI: action=%q err=%v", action, err)
	}

	action, opts = "", CLIOptions{}
	if err := app.Execute([]string{"queue", "ls"}); err == nil && opts.QueueSubcmd == "list" {
		t.Errorf("expected 'pod queue ls' not to resolve to the list subcommand")
	}

	action, opts = "", CLIOptions{}
	if err := app.Execute([]string{"info", "check", "whisper-server"}); err == nil {
		t.Errorf("expected 'pod info check whisper-server' to be rejected; the target is 'whisper'")
	}
}
