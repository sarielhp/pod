package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/sarielhp/clihelp"

	"pod/pkg/podcast"
)

const statusPruneKeep = 5

func buildServerStatusSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "status",
		Description: "Report what the library holds and how much disk space it uses",
		UsageLine:   "pod server status [--top <n>] [--json]",
		Args:        clihelp.MaximumNArgs(0),
		Options: []clihelp.Option{
			clihelp.Int(&opts.StatusTop, "--top <n>", 10, "How many of the largest podcasts to list"),
			clihelp.Bool(&opts.JSON, "--json", false, "Output the figures as JSON"),
		},
		Run: func(_ *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "status"
			return nil
		},
	}
}

func handleServerStatus(cfg Config, cli CLIOptions) error {
	lib := library(cfg, cli, nil)
	if lib.Config().PodcastsDir == "" {
		return fmt.Errorf("podcasts_dir is not configured")
	}
	stats := lib.Stats(podcast.StatsOptions{Top: cli.StatusTop, PruneKeep: statusPruneKeep})
	if cli.JSON {
		return encodeJSON(cli, stats)
	}
	printLibraryStats(outFor(cli), stats)
	return nil
}

func printLibraryStats(w io.Writer, st podcast.LibraryStats) {
	fmt.Fprintf(w, "Library %s\n", st.Root)
	fmt.Fprintf(w, "  Podcasts     %d (%d favorite), %d subscription(s), %d disabled\n", st.Podcasts, st.Favorites, st.Subscriptions, st.DisabledSubs)
	fmt.Fprintf(w, "  Episodes     %d on disk, %d with ads removed, %d with a transcript\n\n", st.Episodes, st.AdsRemoved, st.WithTranscript)

	fmt.Fprintf(w, "Disk use      %s\n", humanBytes(st.TotalBytes))
	fmt.Fprintf(w, "  audio                    %10s\n", humanBytes(st.AudioBytes))
	fmt.Fprintf(w, "  uncut originals          %10s\n", humanBytes(st.OriginalBytes))
	fmt.Fprintf(w, "  transcripts and cuts     %10s\n", humanBytes(st.TranscriptBytes))
	fmt.Fprintf(w, "  work leftovers           %10s\n", humanBytes(st.WorkBytes))
	fmt.Fprintf(w, "  covers, feeds, pages     %10s\n", humanBytes(st.OtherBytes))
	if st.DiskTotal > 0 {
		fmt.Fprintf(w, "  filesystem               %s free of %s\n", humanBytes(int64(st.DiskFree)), humanBytes(int64(st.DiskTotal)))
	}
	printLargest(w, st)
	if st.PrunableBytes > 0 {
		fmt.Fprintf(w, "\nKeeping only the newest %d episodes of each non-favorite podcast would free %s:\n  pod server prune %d --skip-favorites --dry-run\n",
			st.PrunableKeep, humanBytes(st.PrunableBytes), st.PrunableKeep)
	}
}

func printLargest(w io.Writer, st podcast.LibraryStats) {
	if len(st.Largest) == 0 {
		return
	}
	fmt.Fprintf(w, "\nLargest podcasts\n")
	for i, p := range st.Largest {
		mark := " "
		if p.Favorite {
			mark = favoriteMark
		}
		fmt.Fprintf(w, "  %2d. %s %-44s %4d eps  %10s\n", i+1, mark, clip(listTitle(p.Title), 44), p.Episodes, humanBytes(p.Bytes))
	}
}

const favoriteMark = "♥"

// listTitle makes a folder-style name readable. Unlike the TUI it does not reverse
// right-to-left text, which terminals with bidirectional support show correctly.
func listTitle(title string) string {
	return strings.ReplaceAll(title, "_", " ")
}
