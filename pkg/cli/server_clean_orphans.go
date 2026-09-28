package cli

import (
	"fmt"
	"os"
	"pod/pkg/backend"
	"pod/pkg/podcast"

	"github.com/sarielhp/clihelp"
)

func buildServerCleanOrphansSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "clean-orphans",
		Description: "Delete orphaned ABS podcast entries",
		UsageLine:   "pod server clean-orphans [options]",
		Args:        clihelp.NoArgs,
		Options: []clihelp.Option{
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Preview items without deleting"),
			clihelp.Bool(&opts.ForceDelete, "-f, --force", false, "Delete items without confirmation"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed output during pruning"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "clean-orphans"
			opts.SyncSubcmd = "clean-orphans"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func handleServerCleanOrphans(config Config, cli CLIOptions) error {
	b, err := backend.FromAppConfig(&config, reporter(cli))
	if err != nil {
		return fmt.Errorf("podcast server not configured: %w", err)
	}
	opts := podcast.CleanOrphansOptions{
		DryRun:  cli.DryRun,
		Force:   cli.ForceDelete,
		Quiet:   cli.Quiet,
		Verbose: cli.Verbose,
		In:      os.Stdin,
		Out:     outFor(cli),
	}
	_, err = podcast.RunCleanOrphans(b, opts)
	return err
}
