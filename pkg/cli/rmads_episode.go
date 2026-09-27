package cli

import (
	"fmt"
	"path/filepath"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"strconv"
	"strings"
)

func runUrgentEpisode(cfg Config, cli CLIOptions) (bool, error) {
	if cli.ProcSubcmd != "" || cli.Podcast != "" || len(cli.Args) != 1 {
		return false, nil
	}
	id := strings.ToLower(cli.Args[0])
	if len(id) != 6 || id[0] != 'e' {
		return false, nil
	}
	if _, err := strconv.ParseUint(id[1:], 16, 32); err != nil {
		return false, nil
	}
	res, err := resolveQueueTarget(cfg.PodcastsDir, id)
	if err != nil {
		return true, err
	}
	if !res.IsEpisode() {
		return true, fmt.Errorf("%s is not a downloaded episode", id)
	}
	ep := res.Episode
	if cli.DryRun {
		fmt.Fprintf(outFor(cli), "[dry-run] Would queue [%s] %s at priority 10 and process it first.\n", ep.ShortID, ep.Title)
		return true, nil
	}
	item, err := enqueueUrgentEpisode(ep)
	if err != nil {
		return true, err
	}
	cli.Priority = 10
	cli.Normalize()
	return true, executeQueueRun([]queueEpisodeItem{item}, cli, cfg)
}

func enqueueUrgentEpisode(ep *ResolvedEpisode) (queueEpisodeItem, error) {
	item := queueEpisodeItem{PodcastID: ep.PodcastShortID, EpisodeID: ep.ShortID, Title: ep.Title, AudioPath: ep.Path, PodcastDir: ep.PodcastDir, Filename: podcast.QueueFilename(ep.PodcastDir, ep.Path), Priority: 10}
	path, err := episode.ResolveQueueAudioPath(ep.PodcastDir, item.Filename)
	if err != nil || filepath.Clean(path) != filepath.Clean(ep.Path) {
		return item, fmt.Errorf("episode audio is not available in its podcast: %s", ep.ShortID)
	}
	err = episode.UpdateQueue(ep.PodcastDir, func(entries []string) []string {
		ordered := []string{item.Filename}
		for _, entry := range entries {
			resolved, resolveErr := episode.ResolveQueueAudioPath(ep.PodcastDir, entry)
			if resolveErr == nil && filepath.Clean(resolved) == filepath.Clean(ep.Path) {
				continue
			}
			ordered = append(ordered, entry)
		}
		return ordered
	})
	if err != nil {
		return item, err
	}
	st := episode.GetOrCreateEpisodeStatus(ep.Path)
	st.Priority = 10
	err = episode.SaveEpisodeStatus(episode.StatusPathFor(ep.Path), st)
	return item, err
}
