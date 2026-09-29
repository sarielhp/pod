package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"pod/pkg/podcast"
)

// pruneLibraryByCount keeps only the newest `keep` audio files of each podcast
// in scope, optionally sparing favorites, and deletes the rest: the audio and
// its uncut original. Transcripts, cuts and status files stay, so they can
// still feed `pod analyze` and `pod repeats`.
func pruneLibraryByCount(cfg Config, cli CLIOptions, keep int) error {
	lib := library(cfg, cli, nil)
	if lib.Config().PodcastsDir == "" {
		return fmt.Errorf("podcasts_dir is not configured")
	}
	entries, err := pruneScope(lib, cli)
	if err != nil {
		return err
	}
	plans := lib.PlanKeepLatest(entries, podcast.PruneOptions{Keep: keep, SkipFavorites: cli.SkipFavorites})
	if !reportPrunePlans(cli, plans, keep) {
		return nil
	}
	if cli.DryRun {
		fmt.Fprintln(outFor(cli), "[dry-run] Nothing was deleted.")
		return nil
	}
	if !cli.ForceDelete && !confirmPrune(cli, plans) {
		return nil
	}
	return finishPrune(cli, podcast.ApplyPrune(plans))
}

func pruneScope(lib *podcast.Library, cli CLIOptions) ([]podcast.PodcastDirEntry, error) {
	target := cli.Podcast
	if target == "" || target == "all" {
		return lib.Podcasts(), nil
	}
	group, err := lib.ResolveGroup(target)
	if err != nil {
		return nil, err
	}
	return group.Entries, nil
}

// reportPrunePlans prints what a prune would remove and reports whether there
// is anything to remove.
func reportPrunePlans(cli CLIOptions, plans []podcast.PrunePlan, keep int) bool {
	out := progressFor(cli)
	if cli.DryRun {
		out = outFor(cli)
	}
	var episodes, spared, podcasts int
	var bytes int64
	for _, plan := range plans {
		if plan.Skipped != "" {
			spared++
			continue
		}
		if len(plan.Delete) == 0 {
			continue
		}
		podcasts++
		episodes += len(plan.Delete)
		bytes += plan.Bytes()
		fmt.Fprintf(out, "%s: delete %d of %d, %s\n", listTitle(plan.Title), len(plan.Delete), len(plan.Delete)+plan.Kept, humanBytes(plan.Bytes()))
		listPruneFiles(out, plan, cli)
	}
	if episodes == 0 {
		fmt.Fprintf(out, "Every podcast already has %d or fewer episodes on disk.%s\n", keep, sparedNote(spared))
		return false
	}
	fmt.Fprintf(out, "\n%d episode(s) across %d podcast(s), %s.%s\n", episodes, podcasts, humanBytes(bytes), sparedNote(spared))
	return true
}

func listPruneFiles(w io.Writer, plan podcast.PrunePlan, cli CLIOptions) {
	if !cli.DryRun && !cli.Verbose {
		return
	}
	for _, ep := range plan.Delete {
		fmt.Fprintf(w, "    %s\n", ep.Audio)
	}
}

func sparedNote(spared int) string {
	if spared == 0 {
		return ""
	}
	return fmt.Sprintf(" %d favorite(s) left alone.", spared)
}

// confirmPrune asks before an irreversible deletion. The prompt goes to outFor
// even under --quiet, since reading stdin with nothing shown looks like a hang.
func confirmPrune(cli CLIOptions, plans []podcast.PrunePlan) bool {
	episodes := 0
	for _, plan := range plans {
		episodes += len(plan.Delete)
	}
	fmt.Fprintf(outFor(cli), "Delete the audio of %d episode(s)? Transcripts and cuts are kept. [y/N]: ", episodes)
	line, err := bufio.NewReader(inFor(cli)).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if (err != nil && err != io.EOF) || (answer != "y" && answer != "yes") {
		fmt.Fprintln(outFor(cli), "Aborted. Nothing was deleted.")
		return false
	}
	return true
}

func finishPrune(cli CLIOptions, res podcast.PruneResult) error {
	fmt.Fprintf(progressFor(cli), "Deleted %d episode(s) from %d podcast(s), freed %s. Transcripts and cuts kept.\n",
		res.Episodes, res.Podcasts, humanBytes(res.Bytes))
	for _, failure := range res.Failures {
		fmt.Fprintf(errFor(cli), "! %v\n", failure)
	}
	if len(res.Failures) > 0 {
		return fmt.Errorf("prune finished with %d error(s)", len(res.Failures))
	}
	return nil
}
