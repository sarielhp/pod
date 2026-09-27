package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/util"
	"strings"

	"github.com/sarielhp/clihelp"
)

func buildServerFlushSubcommand(opts *CLIOptions, action *string) clihelp.Command {
	return clihelp.Command{
		Name:        "flush",
		Description: "Remove a podcast's audio and precut copies, keep transcripts, and disable automatic downloads",
		UsageLine:   "pod server flush <podcast-id> [--dry-run]",
		Parameters:  []clihelp.Param{{Name: "<podcast-id>", Description: "Exact podcast ID, local short ID, or title"}},
		Args:        clihelp.ExactArgs(1),
		Options: []clihelp.Option{
			clihelp.Bool(&opts.DryRun, "--dry-run", false, "Preview audio removal and download-policy changes"),
			clihelp.Bool(&opts.Quiet, "-q, --quiet", false, "Suppress progress output"),
		},
		Run: func(ctx *clihelp.Context) error {
			*action = "server"
			opts.ServerSubcmd = "flush"
			opts.Args = ctx.Args
			return nil
		},
	}
}

func handleServerFlush(cfg Config, cli CLIOptions) error {
	if len(cli.Args) != 1 || cfg.PodcastsDir == "" {
		return fmt.Errorf("flush requires one podcast ID and a configured podcasts_dir")
	}
	b, err := backend.FromAppConfig(&cfg, reporter(cli))
	if err != nil {
		return err
	}
	items, err := b.Podcasts()
	if err != nil {
		return err
	}
	item, dir, err := resolveFlushPodcast(cfg.PodcastsDir, cli.Args[0], items)
	if err != nil {
		return err
	}
	return flushPodcastAudio(b, item, dir, cli, library(cfg, cli, b).Queue())
}

func resolveFlushPodcast(root, query string, items []backend.Podcast) (backend.Podcast, string, error) {
	matched, err := podcast.MatchBackendPodcasts(items, query)
	if err != nil {
		return backend.Podcast{}, "", err
	}
	dir, err := flushPodcastDir(root, matched.RelPath)
	if err != nil {
		return backend.Podcast{}, "", err
	}
	return *matched, dir, nil
}

func flushPodcastDir(root, rel string) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "podcasts/")
	if rel == "" || filepath.IsAbs(rel) {
		return "", fmt.Errorf("invalid podcast directory %q", rel)
	}
	dir := filepath.Join(root, filepath.FromSlash(rel))
	check, err := filepath.Rel(root, dir)
	if err != nil || check == "." || check == ".." || strings.HasPrefix(check, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("podcast directory must be strictly below podcasts_dir")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil || realDir != filepath.Join(realRoot, check) {
		return "", fmt.Errorf("podcast directory is missing or contains symlinks: %s", dir)
	}
	return dir, nil
}

func flushAudioFiles(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".mp3") || strings.HasSuffix(name, ".mp3.precut") {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

func flushPodcastAudio(b backend.Backend, item backend.Podcast, dir string, cli CLIOptions, queue *podcast.DownloadQueue) error {
	files, err := flushAudioFiles(dir)
	if err != nil {
		return err
	}
	if cli.DryRun {
		fmt.Fprintf(outFor(cli), "[dry-run] Disable automatic downloads for %s; remove %d audio files; preserve transcripts.\n", item.Media.Metadata.Title, len(files))
		for _, path := range files {
			fmt.Fprintln(outFor(cli), path)
		}
		return nil
	}
	local := config.LoadPodcastConfig(dir, config.PodcastConfig{})
	if err := b.UpdatePodcastSettings(item.ID, false, local.IsAutoCleanupEnabled(), local.AutoCleanupDays); err != nil {
		return fmt.Errorf("disable server downloads before flushing: %w", err)
	}
	local.SetAutoDownload(false)
	if err := config.SavePodcastConfig(dir, local); err != nil {
		return err
	}
	if err := queue.FlushPodcast(item.ID, dir); err != nil {
		return err
	}
	active, err := b.ActiveDownloads(item.ID)
	if err != nil {
		return err
	}
	if len(active) != 0 {
		return fmt.Errorf("downloads disabled, but %d downloads are still active; retry flush when they finish", len(active))
	}
	release, err := lockFlushAudio(files)
	if err != nil {
		return err
	}
	defer release()
	for _, path := range files {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("flush %s: %w", path, err)
		}
	}
	if err := episode.UpdateQueue(dir, func([]string) []string { return nil }); err != nil {
		return err
	}
	fmt.Fprintf(progressFor(cli), "Removed %d audio files from %s; transcripts kept. Automatic downloads disabled. Deleted audio can only be recovered from backups or by downloading it again.\n", len(files), item.Media.Metadata.Title)
	return nil
}

func lockFlushAudio(paths []string) (func(), error) {
	var locks []*util.FileLockWrapper
	seen := make(map[string]bool)
	release := func() {
		for _, lock := range locks {
			lock.Release()
		}
	}
	for _, path := range paths {
		target := strings.TrimSuffix(path, ".precut")
		if seen[target] {
			continue
		}
		seen[target] = true
		if episode.IsEpisodeInRemoteFlight(target) {
			release()
			return nil, fmt.Errorf("episode is processing remotely: %s", target)
		}
		lock, err := util.AcquireFileLock(target)
		if err != nil || lock == nil {
			release()
			return nil, fmt.Errorf("audio is locked or unavailable: %s", path)
		}
		locks = append(locks, lock)
	}
	return release, nil
}
