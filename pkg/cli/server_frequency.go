package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"pod/pkg/backend"
	"pod/pkg/podcast"
	"pod/pkg/util"

	"github.com/sarielhp/clihelp"
)

func buildServerFrequencySubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "frequency",
		Description: "Analyze podcast release cadence and save frequency metadata",
		UsageLine:   "pod library frequency [<podcast>] [options]",
		Parameters: []clihelp.Param{
			{Name: "[<podcast>]", Description: "Optional podcast by name, index, or ID to analyze"},
		},
		Args: clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Filter to a single podcast"),
			clihelp.Bool(&opts.Refresh, "--refresh", false, "Force re-fetching latest 100 episodes from online RSS feed"),
			clihelp.Bool(&opts.DisableHourly, "--disable-hourly", false, "Automatically disable download and ad removal for hourly podcasts"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed metrics (span, days interval, hours interval)"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "library"
			opts.ServerSubcmd = "frequency"
			opts.SyncSubcmd = "frequency"
			opts.Args = ctx.Args
			if len(ctx.Args) > 0 && opts.Podcast == "" {
				opts.Podcast = ctx.Args[0]
			}
			return nil
		},
	}
}

func buildServerDisableHourlySubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "disable-hourly",
		Description: "Disable policy for hourly podcasts",
		UsageLine:   "pod library disable-hourly [options]",
		Parameters: []clihelp.Param{
			{Name: "[<podcast>]", Description: "Podcast name, index, or ID"},
		},
		Args: clihelp.MaximumNArgs(1),
		Options: []clihelp.Option{
			clihelp.String(&opts.Podcast, "-p, --podcast <podcast>", "", "Filter to a single podcast"),
			clihelp.Bool(&opts.Refresh, "--refresh", false, "Re-fetch latest episodes from feed"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress outputs"),
			clihelp.Bool(&opts.Verbose, "-v, --verbose", false, "Show detailed debug information"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "library"
			opts.ServerSubcmd = "disable-hourly"
			opts.SyncSubcmd = "disable-hourly"
			opts.DisableHourly = true
			opts.Args = ctx.Args
			if len(ctx.Args) > 0 && opts.Podcast == "" {
				opts.Podcast = ctx.Args[0]
			}
			return nil
		},
	}
}

func handleServerFrequency(config Config, cli CLIOptions) error {
	b, targetItems, err := collectFrequencyTargetPodcasts(config, cli)
	if err != nil {
		return err
	}
	if len(targetItems) == 0 {
		fmt.Fprintln(progressFor(cli), "No podcasts found to analyze.")
		return nil
	}

	shouldDisable := cli.ServerSubcmd == "disable-hourly" || cli.ServerSubcmd == "disable_hourly" || cli.DisableHourly
	if !cli.Quiet {
		desc := "Analyzing podcast release frequency"
		if shouldDisable {
			desc = "Analyzing podcast frequency and updating hourly policies"
		}
		fmt.Fprintf(outFor(cli), "\n%s (%d podcast(s))...\n\n", desc, len(targetItems))
	}

	results := library(config, cli, b).AnalyzeFrequencies(targetItems, podcast.FrequencyOptions{
		Refresh:       cli.Refresh,
		DisableHourly: shouldDisable,
	})

	if !cli.Quiet {
		printFrequencyTable(outFor(cli), results, cli.Verbose, shouldDisable)
	}
	return nil
}

func handleServerDisableHourly(config Config, cli CLIOptions) error {
	cli.DisableHourly = true
	return handleServerFrequency(config, cli)
}

func collectFrequencyTargetPodcasts(config Config, cli CLIOptions) (backend.Backend, []backend.Podcast, error) {
	b, err := backend.FromAppConfig(&config, reporter(cli))
	if err == nil && b != nil {
		podcasts, pErr := resolveServerTargetPodcasts(b, cli)
		if pErr != nil && errors.Is(pErr, podcast.ErrAmbiguousPodcast) {
			return nil, nil, pErr
		}
		if pErr == nil && len(podcasts) > 0 {
			return b, podcasts, nil
		}
	}

	if config.PodcastsDir != "" {
		podEntries := podcast.ScanPodcastDirs(config.PodcastsDir)
		if cli.Podcast != "" {
			matched, mErr := podcast.MatchLocalPodcasts(podEntries, cli.Podcast)
			if mErr != nil {
				return nil, nil, mErr
			}
			podEntries = []podcast.PodcastDirEntry{*matched}
		}
		var localPodcasts []backend.Podcast
		for _, e := range podEntries {
			var feedURL string
			if cache, _ := podcast.LoadPodcastCache(e.Dir); cache != nil {
				feedURL = cache.FeedURL
			}
			title := e.Title
			if title == "" {
				title = e.FolderName
			}
			localPodcasts = append(localPodcasts, backend.Podcast{
				ID: e.ShortID,
				Media: backend.PodcastMedia{
					Metadata: backend.PodcastMetadata{Title: title, FeedURL: feedURL},
				},
			})
		}
		if len(localPodcasts) > 0 {
			return b, localPodcasts, nil
		}
	}

	if err != nil {
		return nil, nil, fmt.Errorf("podcast server not configured: %w", err)
	}
	return b, nil, nil
}

func printFrequencyTable(w io.Writer, results []podcast.PodcastFreqResult, verbose bool, disableMode bool) {
	cadenceGroups := []struct {
		cadence backend.PodcastCadence
		title   string
	}{
		{backend.CadenceHourly, "HOURLY"},
		{backend.CadenceDaily, "DAILY"},
		{backend.CadenceWeekly, "WEEKLY"},
		{backend.CadenceMonthly, "MONTHLY"},
		{backend.CadenceIntermittent, "INTERMITTENT"},
	}

	grouped := make(map[backend.PodcastCadence][]podcast.PodcastFreqResult)
	var errors []podcast.PodcastFreqResult
	disabledCount := 0

	for _, r := range results {
		if r.Err != nil {
			errors = append(errors, r)
			continue
		}
		if r.Disabled {
			disabledCount++
		}
		c := backend.PodcastCadence(r.Freq.Type)
		grouped[c] = append(grouped[c], r)
	}

	for c := range grouped {
		sort.Slice(grouped[c], func(i, j int) bool {
			return strings.ToLower(grouped[c][i].Title) < strings.ToLower(grouped[c][j].Title)
		})
	}

	fmt.Fprintf(w, "  %-3s │ %-38s │ %-9s │ %-11s │ %-11s │ %s\n",
		"#", "Podcast", "Analyzed", "Episodes/Wk", "Median Int", "Status")
	doubleDivider := strings.Repeat("═", 98)
	fmt.Fprintln(w, "  "+doubleDivider)

	globalIdx := 1
	for _, g := range cadenceGroups {
		list := grouped[g.cadence]
		if len(list) == 0 {
			continue
		}
		printCadenceGroup(w, g.title, list, &globalIdx, verbose, disableMode)
	}

	if len(errors) > 0 {
		printFrequencyErrors(w, errors, &globalIdx)
	}

	fmt.Fprintln(w, "  "+doubleDivider)

	if disableMode {
		fmt.Fprintf(w, "\nCompleted hourly check: %d podcast(s) updated with download_policy=none and ad_removal=none.\n\n", disabledCount)
	} else {
		fmt.Fprintln(w)
	}
}

func printCadenceGroup(w io.Writer, groupTitle string, list []podcast.PodcastFreqResult, globalIdx *int, verbose, disableMode bool) {
	suffix := "s"
	if len(list) == 1 {
		suffix = ""
	}
	headerTitle := fmt.Sprintf("─── %s (%d podcast%s) ", groupTitle, len(list), suffix)
	if len(headerTitle) < 98 {
		headerTitle += strings.Repeat("─", 98-len(headerTitle))
	}
	fmt.Fprintln(w, "  "+headerTitle)

	for _, r := range list {
		status := "saved"
		if r.Disabled {
			status = "disabled (none)"
		} else if r.Freq.Type == string(backend.CadenceHourly) && !disableMode {
			status = "hourly (active)"
		}

		epWk := fmt.Sprintf("%.1f/wk", r.Freq.EpisodesPerWeek)
		if r.Freq.EpisodesPerWeek == 0 && r.Freq.Type == string(backend.CadenceIntermittent) {
			epWk = "-"
		}

		medInt := fmt.Sprintf("%.1fh", r.Freq.MedianHoursInterval)
		if r.Freq.MedianHoursInterval >= 48.0 {
			medInt = fmt.Sprintf("%.1fd", r.Freq.MedianHoursInterval/24.0)
		} else if r.Freq.MedianHoursInterval == 0 && r.Freq.Type == string(backend.CadenceIntermittent) {
			medInt = "-"
		}

		epsStr := fmt.Sprintf("%d eps", r.Freq.EpisodesAnalyzed)
		title := util.TruncateDisplayName(r.Title, 38)
		fmt.Fprintf(w, "  %2d. │ %s │ %-9s │ %-11s │ %-11s │ %s\n",
			*globalIdx, util.PadRight(title, 38), epsStr, epWk, medInt, status)
		*globalIdx++

		if verbose {
			spanDays := r.Freq.AvgDaysInterval * float64(max(1, r.Freq.EpisodesAnalyzed-1))
			fmt.Fprintf(w, "       ↳ Span: %.1fd | Avg Interval: %.1fd | Median: %.1fh | Analyzed: %s\n",
				spanDays,
				r.Freq.AvgDaysInterval,
				r.Freq.MedianHoursInterval,
				r.Freq.AnalyzedAt.Format("2006-01-02 15:04:05 UTC"))
		}
	}
	fmt.Fprintln(w, "  "+strings.Repeat("─", 98))
}

func printFrequencyErrors(w io.Writer, errors []podcast.PodcastFreqResult, globalIdx *int) {
	headerTitle := fmt.Sprintf("─── ERRORS (%d) ", len(errors))
	if len(headerTitle) < 98 {
		headerTitle += strings.Repeat("─", 98-len(headerTitle))
	}
	fmt.Fprintln(w, "  "+headerTitle)
	for _, r := range errors {
		title := util.TruncateDisplayName(r.Title, 38)
		fmt.Fprintf(w, "  %2d. │ %s │ %-9s │ %-11s │ %-11s │ %s\n",
			*globalIdx, util.PadRight(title, 38), "-", "-", "-", r.Err.Error())
		*globalIdx++
	}
	fmt.Fprintln(w, "  "+strings.Repeat("─", 98))
}
