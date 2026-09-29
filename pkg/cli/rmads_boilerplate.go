package cli

import (
	"fmt"
	"os"
	"sort"

	"pod/pkg/adremoval"
	"pod/pkg/podcast"
	"pod/pkg/util"
)

// runBoilerplateRecut refreshes episodes with their podcast's recorded
// boilerplate. It is kept apart from the ordinary rm_ads path on purpose: given a
// podcast, that path queues the latest uncleaned episode for ad removal, which a
// recut must never do.
func runBoilerplateRecut(config Config, cli CLIOptions) error {
	targets, err := boilerplateRecutTargets(config, cli)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("no episodes found to recut")
	}
	_, err = adremoval.ProcessFiles(targets, cli.ProcOptions, config, reporter(cli))
	if !cli.DryRun {
		refreshFeedsForAudio(targets, config)
	}
	return err
}

// boilerplateRecutTargets turns the arguments into episode audio files. A
// directory or a podcast name stands for every episode of that podcast.
func boilerplateRecutTargets(config Config, cli CLIOptions) ([]string, error) {
	args := append([]string(nil), cli.Args...)
	if cli.Podcast != "" {
		args = append(args, cli.Podcast)
	}
	var targets []string
	for _, arg := range uniquePaths(args) {
		found, err := episodesFor(config, arg)
		if err != nil {
			return nil, err
		}
		targets = append(targets, found...)
	}
	return targets, nil
}

func episodesFor(config Config, arg string) ([]string, error) {
	fi, err := os.Stat(arg)
	switch {
	case err == nil && fi.IsDir():
		return sortedEpisodes(arg), nil
	case err == nil:
		return []string{arg}, nil
	}
	resolved, err := podcast.ResolveAnyID(config.PodcastsDir, arg)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", arg, err)
	}
	if resolved.IsPodcast() {
		return sortedEpisodes(resolved.Podcast.Dir), nil
	}
	return nil, fmt.Errorf("%s does not name a podcast or an audio file", arg)
}

func sortedEpisodes(dir string) []string {
	files := util.FindMP3Files(dir)
	sort.Strings(files)
	return files
}
