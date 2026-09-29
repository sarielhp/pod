package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"pod/pkg/podcast"

	"github.com/sarielhp/clihelp"
)

func buildServerRewindSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "rewind",
		Description: "Delete episodes downloaded within a time window, with their transcripts and cuts, and reset their feeds so 'pod fetch' finds them new again",
		UsageLine:   "pod server rewind <window> [podcast] [--dry-run] [--force]",
		Parameters: []clihelp.Param{
			{Name: "<window>", Description: "How far back to reach: 90m, 24h, 7d"},
			{Name: "[podcast]", Description: "Limit the rewind to one podcast by name, index, or ID"},
		},
		Args: clihelp.RangeArgs(1, 2),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Limit the rewind to one podcast"),
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "List what would be deleted without deleting it"),
			clihelp.Bool(&opts.ForceDelete, "-f, --force", false, "Delete without asking for confirmation"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "List every file"),
		},
		Examples: []clihelp.Example{
			{Line: "pod server rewind 24h --dry-run", Description: "Preview everything downloaded in the last day"},
			{Line: "pod server rewind 2d 'Huberman Lab'", Description: "Rewind one podcast by two days, then run pod fetch again"},
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "rewind"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func handleServerRewind(cfg Config, cli CLIOptions) error {
	if len(cli.Args) == 0 {
		return fmt.Errorf("rewind requires a window such as 24h or 7d")
	}
	window, err := podcast.ParseRewindWindow(cli.Args[0])
	if err != nil {
		return err
	}
	lib := library(cfg, cli, nil)
	if lib.Config().PodcastsDir == "" {
		return fmt.Errorf("podcasts_dir is not configured")
	}
	store, err := lib.Subscriptions()
	if err != nil {
		return err
	}
	plans := lib.PlanRewind(store.List(), podcast.RewindOptions{Since: window, Target: rewindTarget(cli)})
	if !reportRewindPlans(plans, cli) {
		return nil
	}
	if cli.DryRun {
		fmt.Fprintln(outFor(cli), "[dry-run] Nothing was deleted.")
		return nil
	}
	if !cli.ForceDelete && !confirmRewind(cli, plans) {
		return nil
	}
	return finishRewind(lib.ApplyRewind(plans), cli)
}

func rewindTarget(cli CLIOptions) string {
	if cli.Podcast != "" {
		return cli.Podcast
	}
	if len(cli.Args) > 1 {
		return cli.Args[1]
	}
	return ""
}

// reportRewindPlans prints what a rewind would remove and reports whether there
// is anything to remove.
func reportRewindPlans(plans []podcast.RewindPlan, cli CLIOptions) bool {
	out := progressFor(cli)
	if cli.DryRun {
		out = outFor(cli)
	}
	found := false
	for _, plan := range plans {
		if plan.Err != nil {
			fmt.Fprintf(errFor(cli), "! %s: %v\n", plan.Sub.Title, plan.Err)
			continue
		}
		for _, skipped := range plan.Skipped {
			fmt.Fprintf(errFor(cli), "! %s: skipping %s\n", plan.Sub.Title, skipped)
		}
		if len(plan.Episodes) == 0 {
			continue
		}
		found = true
		fmt.Fprintf(out, "%s: %d episode(s), %s\n", plan.Sub.Title, len(plan.Episodes), humanBytes(plan.Bytes()))
		listRewindEpisodes(plan, cli)
	}
	if !found {
		fmt.Fprintln(out, "No episodes were downloaded in that window.")
	}
	return found
}

func listRewindEpisodes(plan podcast.RewindPlan, cli CLIOptions) {
	if !cli.DryRun && !cli.Verbose {
		return
	}
	out := outFor(cli)
	for _, ep := range plan.Episodes {
		fmt.Fprintf(out, "  %s  (%s)\n", ep.Audio, ep.ModTime.Format("2006-01-02 15:04"))
		if !cli.Verbose {
			continue
		}
		for _, file := range ep.Files {
			if file != ep.Audio {
				fmt.Fprintf(out, "    %s\n", file)
			}
		}
	}
}

// confirmRewind asks before an irreversible deletion. The prompt goes to outFor
// even under --quiet, since reading stdin with nothing shown looks like a hang.
func confirmRewind(cli CLIOptions, plans []podcast.RewindPlan) bool {
	episodes := 0
	for _, plan := range plans {
		episodes += len(plan.Episodes)
	}
	fmt.Fprintf(outFor(cli), "Delete %d episode(s) with their transcripts and cuts, and reset their feeds? [y/N]: ", episodes)
	line, err := bufio.NewReader(inFor(cli)).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if (err != nil && err != io.EOF) || (answer != "y" && answer != "yes") {
		fmt.Fprintln(outFor(cli), "Aborted. Nothing was deleted.")
		return false
	}
	return true
}

func finishRewind(res podcast.RewindResult, cli CLIOptions) error {
	fmt.Fprintf(progressFor(cli), "Rewound %d episode(s) across %d podcast(s): removed %d file(s), freed %s. Run 'pod fetch' to download them again.\n",
		res.Episodes, res.Podcasts, res.Files, humanBytes(res.Bytes))
	for _, failure := range res.Failures {
		fmt.Fprintf(errFor(cli), "! %v\n", failure)
	}
	if len(res.Failures) > 0 {
		return fmt.Errorf("rewind finished with %d error(s)", len(res.Failures))
	}
	return nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
