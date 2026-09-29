package adremoval

import (
	"fmt"
	"time"

	"pod/pkg/audio"
	"pod/pkg/episode"
	"pod/pkg/format"
	"pod/pkg/progress"
	"pod/pkg/types"
	"pod/pkg/util"
)

// BoilerplateReport totals a boilerplate recut over many episodes.
type BoilerplateReport struct {
	Total int
	// Recut counts the episodes recut, or that would be in a dry run.
	Recut    int
	AddedSec float64
	Busy     int
	Failures []error
	// Skipped counts the episodes left alone, by the reason.
	Skipped map[string]int
	// Changed lists the audio files that were recut.
	Changed []string
}

// RecutBoilerplate refreshes episodes with their podcast's recorded boilerplate
// (see pod analyze), with no call to any model. Each target is examined under its
// own file lock; those that cannot or need not be recut are counted by reason, and
// only the ones that change are reported one by one. With opts.DryRun nothing is
// written, and opts.Count stops after that many episodes have been recut.
func RecutBoilerplate(targets []string, opts types.ProcOptions, cfg types.Config, rep progress.Reporter) BoilerplateReport {
	r := progress.Or(rep)
	report := BoilerplateReport{Total: len(targets), Skipped: map[string]int{}}
	start := time.Now()
	for _, target := range targets {
		if opts.Count > 0 && report.Recut >= opts.Count {
			break
		}
		recutOneWithBoilerplate(target, len(targets), opts, cfg, r, start, &report)
	}
	return report
}

func recutOneWithBoilerplate(target string, total int, opts types.ProcOptions, cfg types.Config, r progress.Reporter, start time.Time, report *BoilerplateReport) {
	mainMP3, precut, source := episode.ResolveAudioFiles(target, false)
	base := util.StripExt(mainMP3)
	lock, err := util.AcquireFileLock(mainMP3)
	if err != nil {
		report.Failures = append(report.Failures, err)
		return
	}
	if lock == nil {
		report.Busy++
		return
	}
	defer lock.Release()

	duration := audio.GetAudioDuration(source)
	plan, skip := planBoilerplateRecut(mainMP3, precut, base, duration)
	if skip != "" {
		report.Skipped[skip]++
		return
	}
	name := episode.EpisodeTitleFromPath(mainMP3)
	summary := fmt.Sprintf("%s of boilerplate (%d passage(s))", format.FormatClock(plan.preview.AddedSec), len(plan.cuts))
	if opts.DryRun {
		r.Infof("[dry-run] %s: would add %s", name, summary)
	} else {
		r.Infof("%s: adding %s", name, summary)
		output := episode.ResolveOutputFile(mainMP3, opts.Output, total)
		if err := applyBoilerplateRecut(mainMP3, precut, source, output, base, duration, plan, cfg, opts, start, r); err != nil {
			report.Failures = append(report.Failures, fmt.Errorf("%s: %w", name, err))
			return
		}
	}
	report.Recut++
	report.AddedSec += plan.preview.AddedSec
	report.Changed = append(report.Changed, mainMP3)
}
