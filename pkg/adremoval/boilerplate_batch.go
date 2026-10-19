package adremoval

import (
	"fmt"
	"time"

	"pod/pkg/audio"
	"pod/pkg/episode"
	"pod/pkg/format"
	"pod/pkg/pipeline"
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
	run := &recutRun{
		total:  len(targets),
		opts:   opts,
		cfg:    cfg,
		r:      progress.Or(rep),
		start:  time.Now(),
		report: BoilerplateReport{Total: len(targets), Skipped: map[string]int{}},
		memo:   pipeline.NewRecutMemo(),
	}
	for _, target := range targets {
		if opts.Count > 0 && run.report.Recut >= opts.Count {
			break
		}
		run.examine(target)
	}
	if !opts.DryRun {
		if err := run.memo.Save(); err != nil {
			run.r.Infof("could not remember which episodes need no recut: %v", err)
		}
	}
	return run.report
}

type recutRun struct {
	total  int
	opts   types.ProcOptions
	cfg    types.Config
	r      progress.Reporter
	start  time.Time
	report BoilerplateReport
	memo   *pipeline.RecutMemo
}

func (run *recutRun) examine(target string) {
	mainMP3, precut, source := episode.ResolveAudioFiles(target, false)
	key := run.memo.Key(mainMP3, precut)
	if reason, ok := run.memo.Settled(mainMP3, key); ok {
		run.report.Skipped[reason]++
		return
	}
	lock, err := util.AcquireFileLock(mainMP3)
	if err != nil {
		run.report.Failures = append(run.report.Failures, err)
		return
	}
	if lock == nil {
		run.report.Busy++
		return
	}
	defer lock.Release()

	duration := audio.GetAudioDuration(source)
	plan, skip := planBoilerplateRecut(mainMP3, precut, util.StripExt(mainMP3), duration)
	if skip != "" {
		run.report.Skipped[skip]++
		if settledSkips[skip] {
			run.memo.Settle(mainMP3, key, skip)
		}
		return
	}
	run.recut(mainMP3, precut, source, duration, plan)
}

func (run *recutRun) recut(mainMP3, precut, source string, duration float64, plan boilerplateRecutPlan) {
	name := episode.EpisodeTitleFromPath(mainMP3)
	summary := fmt.Sprintf("%s of boilerplate (%d passage(s))", format.FormatClock(plan.preview.AddedSec), len(plan.cuts))
	if run.opts.DryRun {
		run.r.Infof("[dry-run] %s: would add %s", name, summary)
	} else {
		run.r.Infof("%s: adding %s", name, summary)
		output := episode.ResolveOutputFile(mainMP3, run.opts.Output, run.total)
		if err := applyBoilerplateRecut(mainMP3, precut, source, output, util.StripExt(mainMP3), duration, plan, run.cfg, run.opts, run.start, run.r); err != nil {
			run.report.Failures = append(run.report.Failures, fmt.Errorf("%s: %w", name, err))
			return
		}
	}
	run.report.Recut++
	run.report.AddedSec += plan.preview.AddedSec
	run.report.Changed = append(run.report.Changed, mainMP3)
}
