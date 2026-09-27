package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

type podcastStatusEntry struct {
	id             string
	name           string
	episodes       int
	needsAdRemoval int
}

func absStatus(w io.Writer, cfg Config, showDetailed bool, quiet bool) {
	if showDetailed {
		renderLocalLibraryStatus(w, cfg, quiet)
		return
	}
	renderLocalSummary(w, cfg, quiet)
}

func renderLocalSummary(w io.Writer, cfg Config, quiet bool) (int, int, int) {
	podcastsDir := cfg.PodcastsDir
	if podcastsDir == "" {
		podcastsDir = "."
	}

	podcastsCount := 0
	totalEpisodes := 0
	totalNeedsAd := 0

	dirEntries, err := os.ReadDir(podcastsDir)
	if err == nil {
		for _, de := range dirEntries {
			if !de.IsDir() || strings.HasPrefix(de.Name(), ".") || de.Name() == ".work" || strings.HasSuffix(de.Name(), "-1") {
				continue
			}
			podPath := filepath.Join(podcastsDir, de.Name())
			mp3s := util.FindMP3Files(podPath)
			if len(mp3s) == 0 {
				continue
			}
			podcastsCount++
			totalEpisodes += len(mp3s)
			podCfg := config.LoadPodcastConfig(podPath, config.DefaultDiscoveredPodcastConfig(&cfg))
			if config.NormalizeAdRemovalMode(podCfg.AdRemoval) == AdRemovalNone {
				continue
			}
			filtered := podcast.FilterByAdRemovalPolicy(mp3s, podPath, podCfg)
			for _, mp3 := range filtered {
				_ = episode.GetOrCreateEpisodeStatus(mp3)
				if !episode.IsEpisodeCompleted(mp3) {
					totalNeedsAd++
				}
			}
		}
	}

	if !quiet {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "=== Local Library Status ===")
		fmt.Fprintf(w, "  - Version:           %s\n", getVersion())
		fmt.Fprintf(w, "  - Podcasts:          %d\n", podcastsCount)
		fmt.Fprintf(w, "  - Total Episodes:    %d\n", totalEpisodes)
		if totalNeedsAd > 0 {
			fmt.Fprintf(w, "  - AdR Status:        %s\n", util.BoldYellow(fmt.Sprintf("%d episode(s) need AdR", totalNeedsAd)))
		} else {
			fmt.Fprintf(w, "  - AdR Status:        %s\n", util.BoldGreen("0 (All clean)"))
		}
	}
	return podcastsCount, totalEpisodes, totalNeedsAd
}

func renderLocalLibraryStatus(w io.Writer, cfg Config, quiet bool) {
	podcastsDir := cfg.PodcastsDir
	if podcastsDir == "" {
		podcastsDir = "."
	}
	renderLocalDiskPodcastStatus(w, podcastsDir, cfg, quiet)
}

func renderLocalDiskPodcastStatus(w io.Writer, podcastsDir string, cfg Config, quiet bool) {
	podEntries := podcast.ScanPodcastDirs(podcastsDir)
	var entries []podcastStatusEntry
	for _, pe := range podEntries {
		mp3s := util.FindMP3Files(pe.Dir)
		if len(mp3s) == 0 {
			continue
		}
		needsAd := 0
		podCfg := config.LoadPodcastConfig(pe.Dir, config.DefaultDiscoveredPodcastConfig(&cfg))
		if config.NormalizeAdRemovalMode(podCfg.AdRemoval) != config.AdRemovalNone {
			filtered := podcast.FilterByAdRemovalPolicy(mp3s, pe.Dir, podCfg)
			for _, mp3 := range filtered {
				_ = episode.GetOrCreateEpisodeStatus(mp3)
				if !episode.IsEpisodeCompleted(mp3) {
					needsAd++
				}
			}
		}
		entries = append(entries, podcastStatusEntry{
			id:             pe.ShortID,
			name:           pe.Title,
			episodes:       len(mp3s),
			needsAdRemoval: needsAd,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].name) < strings.ToLower(entries[j].name)
	})

	if quiet {
		return
	}

	fmt.Fprintf(w, "\n%s\n", strings.Repeat("=", 90))
	fmt.Fprintln(w, "LOCAL LIBRARY PODCAST STATUS REPORT")
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 90))
	fmt.Fprintf(w, "  %-3s  %-6s  %-48s │ %-8s │ %-10s\n", "#", "ID", "Title", "Episodes", "NeedAdR")
	fmt.Fprintf(w, "  %-3s  %-6s  %-48s ┼ %-8s ┼ %-10s\n", strings.Repeat("─", 3), strings.Repeat("─", 6), strings.Repeat("─", 48), strings.Repeat("─", 8), strings.Repeat("─", 10))

	totalEpisodes := 0
	totalNeedsAdRemoval := 0

	for idx, e := range entries {
		dName := util.TruncateDisplayName(e.name, 48)
		totalEpisodes += e.episodes
		totalNeedsAdRemoval += e.needsAdRemoval
		fmt.Fprintf(w, "  %-3d  %-6s  %s │ %-8d │ %-16d\n", idx+1, e.id, util.PadRight(dName, 48), e.episodes, e.needsAdRemoval)
	}

	fmt.Fprintf(w, "  %-3s  %-6s  %-48s ┼ %-8s ┼ %-16s\n", strings.Repeat("─", 3), strings.Repeat("─", 6), strings.Repeat("─", 48), strings.Repeat("─", 8), strings.Repeat("─", 16))
	fmt.Fprintf(w, "  %-3s  %-6s  %-48s │ %-8d │ %-16d\n", "", "", "TOTAL", totalEpisodes, totalNeedsAdRemoval)
	fmt.Fprintf(w, "%s\n\n", strings.Repeat("=", 90))
}

func runStatusCommand(config *Config, cli CLIOptions) error {
	if cli.StatusSubcmd == "check" {
		return runCheckCommand(*config, cli)
	}
	showDetailedPodcasts := false
	targetDir := config.PodcastsDir
	if len(cli.Args) > 0 {
		arg := strings.ToLower(cli.Args[0])
		if arg == "podcasts" || arg == "podcast" || arg == "all" {
			showDetailedPodcasts = true
		} else if fi, err := os.Stat(cli.Args[0]); err == nil && fi.IsDir() {
			targetDir = cli.Args[0]
			showDetailedPodcasts = true
		}
	}
	if cli.Verbose {
		showDetailedPodcasts = true
	}
	if targetDir != "" {
		config.PodcastsDir = targetDir
	}
	absStatus(outFor(cli), *config, showDetailedPodcasts, cli.Quiet)
	return nil
}
