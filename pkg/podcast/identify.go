package podcast

import (
	"path/filepath"
	"sort"

	"pod/pkg/backend"
	"pod/pkg/episode"
	"pod/pkg/types"
	"pod/pkg/util"
)

// IdentifyResult reports what identifying one podcast's episodes did.
type IdentifyResult struct {
	Dir   string
	Title string
	Total int
	// Audioless are the episodes among them whose audio is gone and only the
	// transcript, cuts or status remain.
	Audioless int
	Already   int
	// FromFeed were matched to an episode of the feed, by the name they were given.
	FromFeed int
	// Synthetic could not be found in the feed and were given an identity made
	// locally, built from what is known of the file.
	Synthetic int
	// FeedErr is why the feed could not be read, if it could not; every unidentified
	// episode is then synthetic.
	FeedErr error
}

// FeedFetcher reads the episodes of a feed.
type FeedFetcher func(feedURL string) ([]backend.FeedEpisode, error)

// IdentifyEpisodes records an identity for every episode in a podcast directory
// that has none, so that nothing has to be inferred from a file's name again. An
// episode is matched to the feed by the name it was downloaded under, which is
// the last time that inference is used; one the feed no longer lists gets a
// synthetic identity built from its recorded title and publication date, so
// every episode ends with a record and the result does not depend on the feed
// being reachable. With dryRun nothing is written.
func IdentifyEpisodes(entry PodcastDirEntry, feedURL string, fetch FeedFetcher, dryRun bool) IdentifyResult {
	res := IdentifyResult{Dir: entry.Dir, Title: entry.Title}
	files := util.FindMP3Files(entry.Dir)
	for _, stem := range audiolessStems(listFileNames(entry.Dir)) {
		files = append(files, filepath.Join(entry.Dir, stem+".mp3"))
		res.Audioless++
	}
	res.Total = len(files)
	var pending []string
	for _, f := range files {
		if episode.LoadIdentity(f) != nil {
			res.Already++
		} else {
			pending = append(pending, f)
		}
	}
	if len(pending) == 0 {
		return res
	}
	matched := map[string]types.EpisodeIdentity{}
	if feedURL != "" && fetch != nil {
		feed, err := fetch(feedURL)
		res.FeedErr = err
		matchToFeed(feedURL, feed, pending, matched)
	}
	for _, f := range pending {
		id, ok := matched[f]
		if ok {
			res.FromFeed++
		} else {
			id = syntheticIdentity(entry, feedURL, f)
			res.Synthetic++
		}
		if !dryRun {
			_ = episode.SaveIdentity(f, id)
		}
	}
	return res
}

// matchToFeed pairs the unidentified files with feed episodes, going through the
// feed and asking a name-only index which file, if any, holds each episode.
func matchToFeed(feedURL string, feed []backend.FeedEpisode, pending []string, out map[string]types.EpisodeIdentity) {
	wanted := map[string]bool{}
	for _, f := range pending {
		wanted[f] = true
	}
	byName := newLocalEpisodes(pending)
	for _, fe := range feed {
		path, ok := byName.Find(fe)
		if !ok || !wanted[path] {
			continue
		}
		if _, taken := out[path]; !taken {
			out[path] = IdentityOfFeedEpisode(fe, feedURL)
		}
	}
}

// syntheticIdentity is the identity of an episode the feed cannot vouch for. Its
// GUID is built from the podcast's short ID and the file's current name, so the
// same file always gets the same one, and it is recorded before anything is
// renamed, so a later rename cannot change it.
func syntheticIdentity(entry PodcastDirEntry, feedURL, path string) types.EpisodeIdentity {
	id := types.EpisodeIdentity{
		GUID:      SyntheticGUID("local", entry.ShortID, filepath.Base(path)),
		Synthetic: true,
		Title:     episode.EpisodeTitleFromPath(path),
		FeedURL:   feedURL,
	}
	if t := GetEpisodePublicationTime(path); !t.IsZero() {
		id.PublishedAt = t.UnixMilli()
	}
	return id
}

// IdentifyLibrary identifies the episodes of every podcast, reading each
// subscribed podcast's feed once, and reports podcasts in name order.
func (l *Library) IdentifyLibrary(entries []PodcastDirEntry, subs []Subscription, fetch FeedFetcher, dryRun bool) []IdentifyResult {
	feedByDir := map[string]string{}
	for _, sub := range subs {
		feedByDir[filepath.Clean(l.PodcastDir(sub))] = sub.FeedURL
	}
	var out []IdentifyResult
	for _, entry := range entries {
		out = append(out, IdentifyEpisodes(entry, feedByDir[filepath.Clean(entry.Dir)], fetch, dryRun))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}
