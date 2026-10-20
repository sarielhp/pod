package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"pod/pkg/podcast"
	"pod/pkg/util"
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
	isArchive := lib.ArchiveOptions().Enabled
	if !reportPrunePlans(cli, plans, keep, isArchive) {
		return nil
	}
	if cli.DryRun {
		action := "deleted"
		if isArchive {
			action = "archived"
		}
		fmt.Fprintf(outFor(cli), "[dry-run] Nothing was %s.\n", action)
		return nil
	}
	if !cli.ForceDelete && !confirmPrune(cli, plans, isArchive) {
		return nil
	}
	return finishPrune(cli, lib.ApplyPrune(plans), isArchive)
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
func reportPrunePlans(cli CLIOptions, plans []podcast.PrunePlan, keep int, isArchive bool) bool {
	out := progressFor(cli)
	if cli.DryRun {
		out = outFor(cli)
	}
	action := "delete"
	if isArchive {
		action = "archive"
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
		fmt.Fprintf(out, "%s: %s %d of %d, %s\n", listTitle(plan.Title), action, len(plan.Delete), len(plan.Delete)+plan.Kept, humanBytes(plan.Bytes()))
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

// confirmPrune asks before an irreversible deletion or move to archive. The prompt goes to outFor
// even under --quiet, since reading stdin with nothing shown looks like a hang.
func confirmPrune(cli CLIOptions, plans []podcast.PrunePlan, isArchive bool) bool {
	episodes := 0
	for _, plan := range plans {
		episodes += len(plan.Delete)
	}
	prompt := fmt.Sprintf("Delete the audio of %d episode(s)? Transcripts and cuts are kept. [y/N]: ", episodes)
	if isArchive {
		prompt = fmt.Sprintf("Archive the audio of %d episode(s) to cold storage? Transcripts and cuts are kept. [y/N]: ", episodes)
	}
	fmt.Fprint(outFor(cli), prompt)
	line, err := bufio.NewReader(inFor(cli)).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	if (err != nil && err != io.EOF) || (answer != "y" && answer != "yes") {
		abortAction := "deleted"
		if isArchive {
			abortAction = "archived"
		}
		fmt.Fprintf(outFor(cli), "Aborted. Nothing was %s.\n", abortAction)
		return false
	}
	return true
}

func finishPrune(cli CLIOptions, res podcast.PruneResult, isArchive bool) error {
	action := "Deleted"
	if isArchive {
		action = "Archived"
	}
	fmt.Fprintf(progressFor(cli), "%s %d episode(s) from %d podcast(s), freed %s. Transcripts and cuts kept.\n",
		action, res.Episodes, res.Podcasts, humanBytes(res.Bytes))
	for _, failure := range res.Failures {
		util.FprintError(errFor(cli), "! %v\n", failure)
	}
	if len(res.Failures) > 0 {
		return fmt.Errorf("prune finished with %d error(s)", len(res.Failures))
	}
	return nil
}
