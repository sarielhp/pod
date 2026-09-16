package cli

import (
	"fmt"
	"os"
	"pod/pkg/backend"

	"github.com/sarielhp/clihelp"
)

func buildServerCommand(opts *CLIOptions, action *string, countVal, keepVal *int) clihelp.Command {
	return clihelp.Command{
		Name:        "server",
		Description: "Manage podcast server feeds, downloads, and policies",
		UsageLine:   "pod server [command] [options] [podcast-id]",
		Subcommands: buildServerSubcommands(opts, action, countVal, keepVal),
		Examples: []clihelp.Example{
			{
				Line:        "pod server feeds",
				Description: "Check podcast feeds directly for newly published episodes",
			},
			{
				Line:        "pod server download -p 'Huberman Lab' -k 3",
				Description: "Download the 3 latest episodes for a specific podcast",
			},
			{
				Line:        "pod server opml export podcasts.opml",
				Description: "Export server podcast RSS feeds to an OPML file",
			},
		},
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Specify a podcast by name, index, or ID"),
			clihelp.Int(countVal, "-k, --count <number>", -1, "Explicit number of episodes to download"),
			clihelp.Bool(&opts.DownloadAll, "--all", false, "Download all episodes from entire feed catalog"),
			clihelp.Bool(&opts.NoWait, "--no-wait", false, "Do not wait for download completion"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Show output without executing"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Detailed outputs"),
			clihelp.Bool(&opts.PodcastsOnly, "--podcasts-only", false, "Only scan for podcasts (skip downloads)"),
			clihelp.Bool(&opts.EpisodesOnly, "--episodes-only", false, "Only check episodes (skip podcast scan)"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			if len(ctx.Args) > 0 {
				arg := ctx.Args[0]
				if fi, err := os.Stat(arg); err != nil || !fi.IsDir() {
					if opts.Podcast == "" {
						opts.Podcast = arg
					}
				}
			}
			if *countVal > 0 {
				opts.Count = *countVal
				opts.CountGiven = true
			}
			opts.Args = ctx.Args
			return nil
		},
	}
}

func buildServerSubcommands(opts *CLIOptions, action *string, countVal, keepVal *int) []clihelp.Command {
	return []clihelp.Command{
		buildServerFeedsSubcommand(opts, action),
		buildServerDownloadSubcommand(opts, action, countVal, keepVal),
		buildServerPruneSubcommand(opts, action, keepVal),
		buildServerPolicySubcommand(opts, action),
		buildServerFavoriteSubcommand(opts, action),
		buildServerListSubcommand(opts, action),
		buildServerAddSubcommand(opts, action),
		buildServerRemoveSubcommand(opts, action),
		buildServerRSSGenSubcommand(opts, action),
		buildServerImportSubcommand(opts, action),
		buildServerGetInfoSubcommand(opts, action),
		buildServerRescanSubcommand(opts, action),
		buildServerTimelineSubcommand(opts, action),
		buildServerOPMLSubcommand(opts, action),
		buildServerFrequencySubcommand(opts, action),
		buildServerDisableHourlySubcommand(opts, action),
		buildServerCleanOrphansSubcommand(opts, action),
		buildServerFlushSubcommand(opts, action),
		buildServerPublicationSubcommand(opts, action),
	}
}

func handleServerCommand(config Config, cli CLIOptions) error {
	subcmd := cli.ServerSubcmd
	if subcmd == "" {
		subcmd = cli.SyncSubcmd
	}
	switch subcmd {
	case "":
		showServerUsage()
		return nil
	case "feeds":
		return handleServerFeeds(config, cli)
	case "download":
		return handleServerDownload(config, cli)
	case "add":
		return handleServerAdd(config, cli)
	case "remove":
		return handleServerRemove(config, cli)
	case "rss_gen":
		return handleServerFeed(config, cli)
	case "import":
		return handleServerImport(config, cli)
	case "prune", "keep":
		return handleServerKeep(config, cli)
	case "policy":
		return runPolicyCommand(config, cli)
	case "favorite":
		return handleServerFavorite(config, cli)
	case "list":
		return handleServerList(config, cli)
	case "get-info", "get_info":
		return handleServerGetInfo(config, cli)
	case "rescan":
		return handleServerRescan(config, cli)
	case "timeline":
		return handleServerTimeline(config, cli)
	case "opml":
		return handleServerOPML(config, cli)
	case "frequency":
		return handleServerFrequency(config, cli)
	case "disable-hourly", "disable_hourly":
		return handleServerDisableHourly(config, cli)
	case "clean-orphans":
		return handleServerCleanOrphans(config, cli)
	case "flush":
		return handleServerFlush(config, cli)
	case "publication-sync":
		return handleServerPublication(config, cli)
	default:
		return fmt.Errorf("unknown server subcommand %q", subcmd)
	}
}

func showServerUsage() {
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)
	_ = app.RenderCommand(clihelp.Options{}, "server")
}

func resolveServerTargetPodcasts(b backend.Backend, cli CLIOptions) ([]backend.Podcast, error) {
	podcasts, err := b.Podcasts()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch podcasts from server: %w", err)
	}
	return filterServerTargets(podcasts, cli)
}

// filterServerTargets narrows a backend listing to what the flags name. The
// library root comes from the loaded config rather than being re-read here.
func filterServerTargets(podcasts []backend.Podcast, cli CLIOptions) ([]backend.Podcast, error) {
	return library(loadConfig(), cli, nil).SelectBackendTargets(podcasts, serverTargetName(cli))
}

func serverTargetName(cli CLIOptions) string {
	target := cli.Podcast
	if target == "" && len(cli.Args) > 0 {
		if cli.Args[0] != "update" {
			target = cli.Args[0]
		} else if len(cli.Args) > 1 {
			target = cli.Args[1]
		}
	}
	return target
}
