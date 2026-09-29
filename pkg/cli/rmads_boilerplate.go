package cli

import (
	"fmt"
	"os"
	"sort"

	"pod/pkg/adremoval"
	"pod/pkg/format"
	"pod/pkg/util"
)

// runBoilerplateRecut refreshes episodes with their podcast's recorded
// boilerplate. It is kept apart from the ordinary rm_ads path on purpose: given a
// podcast, that path queues the latest uncleaned episode for ad removal, which a
// recut must never do. With no argument it covers every podcast.
func runBoilerplateRecut(config Config, cli CLIOptions) error {
	targets, err := boilerplateRecutTargets(config, cli)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("no episodes found to recut")
	}
	report := adremoval.RecutBoilerplate(targets, cli.ProcOptions, config, reporter(cli))
	if len(report.Changed) > 0 && !cli.DryRun {
		refreshFeedsForAudio(report.Changed, config)
	}
	return printBoilerplateReport(cli, report)
}

// boilerplateRecutTargets turns the arguments into episode audio files. A
// directory or a podcast name stands for every episode of that podcast, an audio
// file for itself, and no argument at all for every episode of every podcast. They
// come newest first by publication date, across podcasts, so that a limit such as
// -n 20 means the twenty newest episodes.
func boilerplateRecutTargets(config Config, cli CLIOptions) ([]string, error) {
	args := append([]string(nil), cli.Args...)
	if cli.Podcast != "" {
		args = append(args, cli.Podcast)
	}
	var files, names []string
	for _, arg := range uniquePaths(args) {
		if fi, err := os.Stat(arg); err == nil && !fi.IsDir() {
			files = append(files, arg)
		} else {
			names = append(names, arg)
		}
	}
	if len(args) > 0 && len(names) == 0 {
		return files, nil
	}
	scope := cli
	scope.Args = names
	dirs, err := missingTargets(config, scope)
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		files = append(files, util.FindMP3Files(dir)...)
	}
	sortFilesByPublicationTime(files)
	return files, nil
}

func printBoilerplateReport(cli CLIOptions, r adremoval.BoilerplateReport) error {
	w := outFor(cli)
	verb := "Recut"
	if cli.DryRun {
		verb = "Would recut"
	}
	fmt.Fprintf(w, "\n%s %d of %d episode(s), adding %s of boilerplate.\n", verb, r.Recut, r.Total, format.FormatClock(r.AddedSec))
	reasons := make([]string, 0, len(r.Skipped))
	for reason := range r.Skipped {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool {
		if r.Skipped[reasons[i]] != r.Skipped[reasons[j]] {
			return r.Skipped[reasons[i]] > r.Skipped[reasons[j]]
		}
		return reasons[i] < reasons[j]
	})
	for _, reason := range reasons {
		fmt.Fprintf(w, "  %4d skipped: %s\n", r.Skipped[reason], reason)
	}
	if r.Busy > 0 {
		fmt.Fprintf(w, "  %4d busy: another pod process had them; run again to pick them up\n", r.Busy)
	}
	for _, f := range r.Failures {
		util.FprintError(errFor(cli), "%v", f)
	}
	if len(r.Failures) > 0 {
		return fmt.Errorf("%d episode(s) could not be recut and were left as they were", len(r.Failures))
	}
	return nil
}
