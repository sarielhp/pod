package podcast

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	"pod/pkg/backend"
)

type OrphanPodcast struct {
	Item             backend.Podcast
	Reason           string
	DuplicateOfID    string
	DuplicateOfTitle string
	EpisodeCount     int
}

type CleanOrphansOptions struct {
	DryRun  bool
	Force   bool
	Quiet   bool
	Verbose bool
	In      io.Reader
	Out     io.Writer
}

type CleanOrphansResult struct {
	ScannedCount int
	OrphanCount  int
	DeletedCount int
	FailedCount  int
	Orphans      []OrphanPodcast
	Errors       []error
}

func normalizeFeedURL(u string) string {
	trimmed := strings.TrimSpace(u)
	trimmed = strings.TrimRight(trimmed, "/")
	return strings.ToLower(trimmed)
}

func findOrphanPodcasts(podcasts []backend.Podcast) []OrphanPodcast {
	var orphans []OrphanPodcast
	orphanedIDs := make(map[string]bool)

	for _, item := range podcasts {
		feedURL := strings.TrimSpace(item.Media.Metadata.FeedURL)
		if feedURL == "" {
			orphans = append(orphans, OrphanPodcast{
				Item:         item,
				Reason:       "Missing or empty RSS feed URL",
				EpisodeCount: len(item.Media.Episodes),
			})
			orphanedIDs[item.ID] = true
		}
	}

	feedGroups := make(map[string][]backend.Podcast)
	for _, item := range podcasts {
		if orphanedIDs[item.ID] {
			continue
		}
		normURL := normalizeFeedURL(item.Media.Metadata.FeedURL)
		if normURL != "" {
			feedGroups[normURL] = append(feedGroups[normURL], item)
		}
	}

	dupOrphans := collectDuplicateFeedOrphans(feedGroups, orphanedIDs)
	orphans = append(orphans, dupOrphans...)
	return orphans
}

func collectDuplicateFeedOrphans(feedGroups map[string][]backend.Podcast, orphanedIDs map[string]bool) []OrphanPodcast {
	var orphans []OrphanPodcast
	for _, group := range feedGroups {
		if len(group) <= 1 {
			continue
		}
		sortFeedGroup(group)
		primary := group[0]
		primaryTitle := primary.Media.Metadata.Title
		if primaryTitle == "" {
			primaryTitle = "Untitled"
		}

		for _, dup := range group[1:] {
			if orphanedIDs[dup.ID] {
				continue
			}
			orphans = append(orphans, OrphanPodcast{
				Item:             dup,
				Reason:           fmt.Sprintf("Duplicate feed URL (matches %q, keeping ID %s with %d episode(s))", primaryTitle, primary.ID, len(primary.Media.Episodes)),
				DuplicateOfID:    primary.ID,
				DuplicateOfTitle: primaryTitle,
				EpisodeCount:     len(dup.Media.Episodes),
			})
			orphanedIDs[dup.ID] = true
		}
	}
	return orphans
}

func sortFeedGroup(group []backend.Podcast) {
	sort.SliceStable(group, func(i, j int) bool {
		if len(group[i].Media.Episodes) != len(group[j].Media.Episodes) {
			return len(group[i].Media.Episodes) > len(group[j].Media.Episodes)
		}
		scoreI := podcastItemScore(group[i])
		scoreJ := podcastItemScore(group[j])
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return group[i].ID < group[j].ID
	})
}

func podcastItemScore(item backend.Podcast) int {
	score := 0
	if item.Media.CoverPath != "" {
		score++
	}
	if item.Media.Metadata.Author != "" {
		score++
	}
	if item.Media.Metadata.Description != "" {
		score++
	}
	return score
}

func RunCleanOrphans(client backend.Backend, opts CleanOrphansOptions) (CleanOrphansResult, error) {
	if client == nil {
		return CleanOrphansResult{}, fmt.Errorf("backend client is nil")
	}
	if opts.In == nil {
		opts.In = strings.NewReader("")
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}

	if !opts.Quiet {
		fmt.Fprintln(opts.Out, "Scanning library for orphaned podcasts...")
	}

	podcasts, err := client.Podcasts()
	if err != nil {
		return CleanOrphansResult{}, fmt.Errorf("failed to fetch podcasts: %w", err)
	}

	orphans := findOrphanPodcasts(podcasts)
	res := CleanOrphansResult{
		ScannedCount: len(podcasts),
		OrphanCount:  len(orphans),
		Orphans:      orphans,
	}

	if len(orphans) == 0 {
		if !opts.Quiet {
			fmt.Fprintf(opts.Out, "No orphaned or fake podcast entries found (scanned %d podcasts).\n", len(podcasts))
		}
		return res, nil
	}

	if !opts.Quiet {
		printOrphansList(opts.Out, orphans, opts.Verbose, len(podcasts))
	}

	if opts.DryRun {
		if !opts.Quiet {
			fmt.Fprintf(opts.Out, "Dry run enabled: %d podcast(s) identified but none were deleted.\n", len(orphans))
		}
		return res, nil
	}

	if !opts.Force {
		confirmed, err := confirmOrphanDeletion(opts.In, opts.Out, len(orphans), opts.Quiet)
		if err != nil || !confirmed {
			return res, err
		}
	}

	executeOrphanDeletions(client, orphans, opts, &res)
	return res, nil
}

func printOrphansList(out io.Writer, orphans []OrphanPodcast, verbose bool, scannedCount int) {
	fmt.Fprintf(out, "\nFound %d orphaned / fake podcast entry(ies) (out of %d scanned):\n", len(orphans), scannedCount)
	for idx, o := range orphans {
		title := o.Item.Media.Metadata.Title
		if title == "" {
			title = "Untitled Podcast"
		}
		fmt.Fprintf(out, "  %d. %s\n", idx+1, title)
		fmt.Fprintf(out, "     ID: %s | Episodes: %d\n", o.Item.ID, o.EpisodeCount)
		fmt.Fprintf(out, "     Reason: %s\n", o.Reason)
		if verbose && o.Item.RelPath != "" {
			fmt.Fprintf(out, "     Path: %s\n", o.Item.RelPath)
		}
	}
	fmt.Fprintln(out)
}

func confirmOrphanDeletion(in io.Reader, out io.Writer, count int, quiet bool) (bool, error) {
	fmt.Fprintf(out, "Are you sure you want to delete these %d orphaned podcast(s)? [y/N]: ", count)
	reader := bufio.NewReader(in)
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("failed to read confirmation: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))
	if input != "y" && input != "yes" {
		if !quiet {
			fmt.Fprintln(out, "Aborted. No podcasts were deleted.")
		}
		return false, nil
	}
	return true, nil
}

func executeOrphanDeletions(client backend.Backend, orphans []OrphanPodcast, opts CleanOrphansOptions, res *CleanOrphansResult) {
	if !opts.Quiet {
		fmt.Fprintln(opts.Out, "Deleting orphaned podcasts...")
	}

	for _, o := range orphans {
		err := client.DeletePodcast(o.Item.ID)
		title := o.Item.Media.Metadata.Title
		if title == "" {
			title = "Untitled Podcast"
		}
		if err != nil {
			res.FailedCount++
			res.Errors = append(res.Errors, err)
			if !opts.Quiet {
				fmt.Fprintf(opts.Out, "  [✗] Failed to delete %q (ID: %s): %v\n", title, o.Item.ID, err)
			}
		} else {
			res.DeletedCount++
			if !opts.Quiet {
				fmt.Fprintf(opts.Out, "  [✓] Deleted %q (ID: %s)\n", title, o.Item.ID)
			}
		}
	}

	if !opts.Quiet {
		if res.FailedCount == 0 {
			fmt.Fprintf(opts.Out, "\nSuccessfully deleted %d orphaned podcast(s).\n", res.DeletedCount)
		} else {
			fmt.Fprintf(opts.Out, "\nDeleted %d orphaned podcast(s), %d failed.\n", res.DeletedCount, res.FailedCount)
		}
	}
}
