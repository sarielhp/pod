package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/types"
	"pod/pkg/util"
)

func groupAndFilterAudioByPodcast(rawMp3Files []string, opts types.ProcOptions, appCfg types.Config) []string {
	filesByFolder := make(map[string][]string)
	for _, f := range rawMp3Files {
		epFolder := filepath.Dir(f)
		if strings.HasSuffix(epFolder, "-1") || strings.HasSuffix(epFolder, "-1/") {
			continue
		}
		folder := episode.DetectPodcastDirForAudio(f)
		filesByFolder[folder] = append(filesByFolder[folder], f)
	}

	var podFolders []string
	for folder := range filesByFolder {
		podFolders = append(podFolders, folder)
	}
	sort.Strings(podFolders)

	var filtered []string
	for _, podFolder := range podFolders {
		fList := filesByFolder[podFolder]
		podCfg := config.LoadPodcastConfig(podFolder, config.DefaultPodcastConfig(&appCfg))
		if !opts.DryRun {
			ensurePodcastConfig(podFolder, podCfg, opts.Quiet)
		}
		if config.NormalizeAdRemovalMode(podCfg.AdRemoval) == config.AdRemovalNone {
			if opts.Verbose && !opts.Quiet {
				fmt.Printf("Podcast config set to 'none' for '%s'. Skipping.\n", filepath.Base(podFolder))
			}
			continue
		}
		filtered = append(filtered, podcast.FilterByAdRemovalPolicy(fList, podFolder, podCfg)...)
	}
	return filtered
}

func expandSingleDirectoryArg(arg string, opts types.ProcOptions, appCfg types.Config) []string {
	if !opts.DryRun {
		removeWorkDirs(arg)
	}
	rawMp3Files := util.FindMP3Files(arg)
	if len(rawMp3Files) == 0 {
		if !opts.Quiet {
			fmt.Printf("No MP3 files found in directory '%s'.\n", arg)
		}
		return nil
	}
	return groupAndFilterAudioByPodcast(rawMp3Files, opts, appCfg)
}

func sortFilesByPublicationTime(files []string) {
	if len(files) <= 1 {
		return
	}
	sort.SliceStable(files, func(i, j int) bool {
		ti := podcast.GetEpisodePublicationTime(files[i])
		tj := podcast.GetEpisodePublicationTime(files[j])
		if ti.Equal(tj) {
			return files[i] < files[j]
		}
		return ti.After(tj)
	})
}

// expandDirectoryArgs resolves directories into their constituent episode audio files.
func expandDirectoryArgs(args []string, opts types.ProcOptions, appCfg types.Config) []string {
	var expandedArgs []string
	hasPrintedScanning := false
	printScanning := func(dir string) {
		if opts.Quiet {
			return
		}
		if !hasPrintedScanning {
			fmt.Println()
			hasPrintedScanning = true
		}
		fmt.Printf("Scanning: %s\n", dir)
	}

	for _, arg := range args {
		fi, err := os.Stat(arg)
		if err == nil && fi.IsDir() {
			printScanning(arg)
			expandedArgs = append(expandedArgs, expandSingleDirectoryArg(arg, opts, appCfg)...)
		} else {
			expandedArgs = append(expandedArgs, arg)
		}
	}

	sortFilesByPublicationTime(expandedArgs)
	return expandedArgs
}

func ensurePodcastConfig(dir string, cfg config.PodcastConfig, quiet bool) {
	if util.FileExists(filepath.Join(dir, config.PodcastConfigFileName)) {
		return
	}
	if cfg.ID == "" {
		cfg.ID = podcast.GetOrSetPodcastShortID(dir, filepath.Base(dir))
	}
	if err := config.SavePodcastConfig(dir, cfg); err != nil {
		if !quiet {
			fmt.Fprintf(os.Stderr, "Warning: could not write default config for %s: %v\n", filepath.Base(dir), err)
		}
		return
	}
	if !quiet {
		fmt.Printf("Created default %s for '%s' (ad removal: %s)\n",
			config.PodcastConfigFileName, filepath.Base(dir), cfg.AdRemoval)
	}
}

func removeWorkDirs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == ".work" {
			_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
		}
	}
}

// refreshFeedsForAudio regenerates the static site for each podcast directory
// holding one of these files.
func refreshFeedsForAudio(audioPaths []string, cfg types.Config) {
	seen := map[string]bool{}
	for _, p := range audioPaths {
		dir := filepath.Dir(p)
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		refreshPodcastFeedXML(dir, cfg)
	}
}
