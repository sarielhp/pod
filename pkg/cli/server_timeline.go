package cli

import (
	"fmt"
	"pod/pkg/backend"
	"pod/pkg/util"

	"github.com/sarielhp/clihelp"
)

func buildServerTimelineSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "timeline",
		Description: "Display exact online availability timestamps table for recent podcast episodes",
		UsageLine:   "pod library timeline [directory]",
		Parameters: []clihelp.Param{
			{Name: "[directory]", Description: "Optional path to podcasts directory"},
		},
		Args: clihelp.MaximumNArgs(1),
		Run: func(ctx *clihelp.Context) error {
			*action = "library"
			opts.ServerSubcmd = "timeline"
			opts.SyncSubcmd = "timeline"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func handleServerTimeline(config Config, cli CLIOptions) error {
	b, err := backend.FromAppConfig(&config, reporter(cli))
	if err != nil {
		return fmt.Errorf("podcast server not configured: %w", err)
	}
	podcasts, err := resolveServerTargetPodcasts(b, cli)
	if err != nil {
		return err
	}
	for _, item := range podcasts {
		fmt.Fprintf(progressFor(cli), "\nTimeline for %s (%d episodes):\n", util.Bold(util.DisplayName(item.Media.Metadata.Title)), len(item.Media.Episodes))
		for i, ep := range item.Media.Episodes {
			if i >= 10 && !cli.Verbose {
				break
			}
			fmt.Fprintf(outFor(cli), "  - %s (%s)\n", util.DisplayName(ep.Title), ep.PubDate)
		}
	}
	return nil
}
