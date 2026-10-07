package cli

import (
	"fmt"
	"pod/pkg/backend"

	"github.com/sarielhp/clihelp"
)

func buildServerRescanSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "rescan",
		Description: "Scan MP3 file lengths on disk against DB duration and update DB if shorter",
		UsageLine:   "pod library rescan [options]",
		Args:        clihelp.NoArgs,
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Specify podcast by index or title"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Preview actions without updating DB"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Detailed trace outputs"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.String(&opts.SqliteDBPath, "--db-path <path>", "", "Path to absdatabase.sqlite"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "library"
			opts.ServerSubcmd = "rescan"
			opts.SyncSubcmd = "rescan"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func handleServerRescan(config Config, cli CLIOptions) error {
	b, err := backend.FromAppConfig(&config, reporter(cli))
	if err != nil {
		return fmt.Errorf("podcast server not configured: %w", err)
	}
	opts := backend.RescanOptions{
		PodcastsDir: config.PodcastsDir,
		PodcastID:   cli.Podcast,
		DryRun:      cli.DryRun,
		Verbose:     cli.Verbose,
		Quiet:       cli.Quiet,
	}
	res, err := b.Rescan(opts)
	if err != nil {
		return fmt.Errorf("rescan failed: %w", err)
	}
	fmt.Fprintf(progressFor(cli), "Rescan completed: checked %d episodes, updated %d.\n", res.CheckedCount, res.RescanCount)
	return nil
}
