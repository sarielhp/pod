package cli

import (
	"fmt"
	"github.com/sarielhp/clihelp"
	"pod/pkg/backend"
	"pod/pkg/podcast"
)

func buildServerPublicationSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "publication-sync",
		Description: "Repair cached and status publication dates from the source catalog",
		Args:        clihelp.MaximumNArgs(0),
		Options:     []clihelp.Option{clihelp.Bool(&opts.DryRun, "--dry-run", false, "Report changes without writing metadata")},
		Run: func(_ *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "publication-sync"
			return nil
		},
	}
}

func handleServerPublication(cfg Config, cli CLIOptions) error {
	if cfg.PodcastsDir == "" {
		return fmt.Errorf("podcasts_dir is not configured")
	}
	b, err := backend.FromAppConfig(&cfg, nil)
	if err != nil {
		return err
	}
	dates, err := podcast.LoadSourcePublicationDates(b, cfg.PodcastsDir)
	if err != nil {
		return err
	}
	statuses, caches, err := podcast.RepairPublicationDates(cfg.PodcastsDir, dates, cli.DryRun)
	if err != nil {
		return err
	}
	verb := "Updated"
	if cli.DryRun {
		verb = "Would update"
	}
	fmt.Fprintf(outFor(cli), "%s publication dates in %d status files and %d podcast caches.\n", verb, statuses, caches)
	return nil
}
