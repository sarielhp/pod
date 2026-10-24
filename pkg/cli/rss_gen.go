package cli

import (
	"fmt"
	"path/filepath"

	"pod/pkg/config"
	"pod/pkg/episode"
	"pod/pkg/podcast"
	"pod/pkg/types"
)

// handleRSSGen republishes feeds, and with an episode argument first recomputes
// that episode's enclosure hash.
//
// The hash is normally written when the cut is produced and the feed is
// regenerated from it; naming the episode is for the case where the audio
// changed after publication. The recomputed digest is republished as the
// enclosure's `?v=` token and its show is republished immediately. The token
// marks which audio a URL refers to; it does not make a client that already
// holds the episode download it again — AntennaPod keeps its file until the
// download is deleted on the device.
func handleRSSGen(cfg Config, cli CLIOptions) error {
	if len(cli.Args) == 0 {
		return republishAllFeeds(cfg, cli)
	}
	return republishOneEpoch(cfg, cli, cli.Args[0])
}

// republishAllFeeds regenerates every show's feed using the archive-aware
// publish path, so archived episodes keep their enclosure URLs.
func republishAllFeeds(cfg Config, cli CLIOptions) error {
	lib := library(cfg, cli, nil)
	store, err := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	if err != nil {
		return fmt.Errorf("open subscriptions store: %w", err)
	}
	subs := store.List()
	if len(subs) == 0 {
		return fmt.Errorf("no subscriptions found in %s", store.FilePath())
	}

	out := outFor(cli)
	for _, sub := range subs {
		if err := lib.Publish(sub, nil); err != nil {
			fmt.Fprintf(errFor(cli), "Warning: failed to write feed for %s: %v\n", sub.Title, err)
			continue
		}
		eps := podcast.CollectLocalEpisodes(lib.PodcastDir(sub), nil)
		fmt.Fprintf(out, "Updated feed: %s (%d episodes)\n", filepath.Join(lib.PodcastDir(sub), "feed.xml"), len(eps))
	}
	return lib.PublishCatalog(subs)
}

// republishOneEpoch resolves one episode, recomputes its content hash, and
// republishes its show. A query that names a show rather than an episode falls
// back to republishing that show, whose episodes gain hashes on the read path.
func republishOneEpoch(cfg Config, cli CLIOptions, query string) error {
	lib := library(cfg, cli, nil)
	res, err := lib.Resolve(query)
	if err != nil {
		return err
	}

	out := outFor(cli)
	if res.IsPodcast() {
		sub, err := subscriptionForPodcast(cfg, res.Podcast.FolderName)
		if err != nil {
			return err
		}
		if err := lib.Publish(*sub, nil); err != nil {
			return err
		}
		fmt.Fprintf(out, "Updated feed: %s\n", filepath.Join(res.Podcast.Dir, "feed.xml"))
		return nil
	}

	ep := res.Episode
	oldHash := ""
	if st, _ := episode.LoadEpisodeStatus(episode.StatusPathFor(ep.Path)); st != nil {
		oldHash = st.Cleaned.ContentHash
	}
	newHash, err := episode.ContentHash(ep.Path)
	if err != nil {
		return fmt.Errorf("hash %s: %w", filepath.Base(ep.Path), err)
	}
	if err := episode.UpdateEpisodeStatus(ep.Path, func(st *types.EpisodeStatusFile) {
		st.Cleaned.ContentHash = newHash
	}); err != nil {
		return fmt.Errorf("record hash for %s: %w", filepath.Base(ep.Path), err)
	}

	sub, err := subscriptionForPodcast(cfg, filepath.Base(ep.PodcastDir))
	if err != nil {
		return err
	}
	if err := lib.Publish(*sub, nil); err != nil {
		return err
	}

	if oldHash == newHash {
		fmt.Fprintf(out, "%s: hash unchanged (%s); feed republished\n", ep.ShortID, newHash)
	} else {
		fmt.Fprintf(out, "%s: enclosure token %s -> %s; feed republished\n", ep.ShortID, tokenOrNone(oldHash), newHash)
	}
	return nil
}

func tokenOrNone(hash string) string {
	if hash == "" {
		return "(none)"
	}
	return hash
}

// subscriptionForPodcast finds the subscription whose folder matches a show
// directory's base name, so publishing goes through the same archive-aware path
// the pipeline uses.
func subscriptionForPodcast(cfg Config, folder string) (*podcast.Subscription, error) {
	store, err := podcast.NewSubscriptionStore(config.SubscriptionsFilePath(&cfg))
	if err != nil {
		return nil, fmt.Errorf("open subscriptions store: %w", err)
	}
	for _, sub := range store.List() {
		if sub.Folder == folder {
			s := sub
			return &s, nil
		}
	}
	return nil, fmt.Errorf("no subscription matches %q", folder)
}
