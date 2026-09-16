package adremoval

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pod/pkg/config"
	"pod/pkg/format"
	"pod/pkg/pipeline"
	"pod/pkg/podcast"
	"pod/pkg/transcribe"
	"pod/pkg/types"
	"pod/pkg/util"
)

// ProcessFiles removes ads from an already-resolved set of targets: audio
// files, or directories to be expanded into the episodes their podcast's
// ad-removal policy admits. Interpreting a command line into that set belongs
// to the caller.
func ProcessFiles(targets []string, opts types.ProcOptions, cfg types.Config, action string) {
	opts.Normalize()

	expandedArgs := expandDirectoryArgs(targets, opts, cfg)
	if len(expandedArgs) == 0 {
		if !opts.Quiet {
			fmt.Println("No files or directories with audio found to process.")
		}
		return
	}

	if opts.DryRun {
		handleProcDryRun(expandedArgs, opts, cfg)
		return
	}

	if err := executeLocalBatchProcessing(expandedArgs, opts, cfg, action); err != nil {
		return
	}
	// Republish whatever was touched. Ad removal by episode id already did
	// this; by file path or directory it did not, so the same work left the
	// published feed correct or stale depending only on how the argument was
	// typed.
	refreshFeedsForAudio(expandedArgs, cfg)
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

func groupAndFilterAudioByPodcast(rawMp3Files []string, opts types.ProcOptions, appCfg types.Config) []string {
	filesByFolder := make(map[string][]string)
	for _, f := range rawMp3Files {
		epFolder := filepath.Dir(f)
		if strings.HasSuffix(epFolder, "-1") || strings.HasSuffix(epFolder, "-1/") {
			continue
		}
		folder := podcast.DetectPodcastDirForAudio(f)
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

func executeLocalBatchProcessing(expandedArgs []string, opts types.ProcOptions, cfg types.Config, action string) error {
	wp := config.GetActiveWhisperProfile(&cfg)
	if opts.WhisperEngine != "" {
		wp.Engine = types.WhisperEngine(opts.WhisperEngine)
	}
	if wp.Engine != types.WhisperEngineLocal && wp.Engine != types.WhisperEngineGemini {
		transcribe.WakeServer(cfg.WhisperURL, cfg.WhisperWakeCommand, opts.Quiet)
	}

	selectedProfile, _ := config.SelectLLMProfile(&cfg, opts.UseLLM)
	batchStartTime := time.Now()

	totalFiles := len(expandedArgs)
	processedCount := 0
	failures := 0

	for idx, inputFile := range expandedArgs {
		hasError, processedFlag, stopFlag := processSingleAudioFile(idx, len(expandedArgs), processedCount, inputFile, opts, cfg, action, batchStartTime, selectedProfile)
		if hasError || (!processedFlag && !stopFlag && (opts.ForceTranscribe || opts.ForceLLM || opts.Recut || !pipeline.IsEpisodeClean(inputFile))) {
			failures++
		}
		if stopFlag {
			break
		}
		if processedFlag {
			processedCount++
		}
	}

	if (processedCount > 1 || totalFiles > 1) && !opts.Quiet {
		batchDuration := time.Since(batchStartTime)
		fmt.Printf("\nBatch Completed! Processed %d file(s) in %s.\n", processedCount, format.FormatClock(batchDuration.Seconds()))
	}

	os.Stdout.Sync()
	os.Stderr.Sync()
	if failures > 0 {
		return fmt.Errorf("%d episode(s) failed or could not be processed", failures)
	}
	return nil
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
		if entry.IsDir() {
			if entry.Name() == ".work" {
				_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
			} else {
				removeWorkDirs(filepath.Join(dir, entry.Name()))
			}
		}
	}
}
