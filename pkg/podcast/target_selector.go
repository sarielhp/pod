package podcast

import (
	"fmt"
	"strings"

	"pod/pkg/backend"
	"pod/pkg/config"
)

type PodcastGroupKind string

const (
	GroupKindNone         PodcastGroupKind = ""
	GroupKindAll          PodcastGroupKind = "all"
	GroupKindFavorites    PodcastGroupKind = "favorites"
	GroupKindNonFavorites PodcastGroupKind = "non-favorites"
	GroupKindSingle       PodcastGroupKind = "single"
)

func parsePodcastGroupKind(query string) (PodcastGroupKind, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	switch q {
	case "all", "*":
		return GroupKindAll, true
	case "fav", "favs", "favorite", "favorites":
		return GroupKindFavorites, true
	case "not-fav", "not-favs", "not-favorite", "not-favorites",
		"non-fav", "non-favs", "non-favorite", "non-favorites", "unfav":
		return GroupKindNonFavorites, true
	default:
		return GroupKindNone, false
	}
}

type ResolvedPodcastGroup struct {
	Kind    PodcastGroupKind
	Label   string
	Entries []PodcastDirEntry
}

func ResolvePodcastGroup(podcastsDir, query string) (*ResolvedPodcastGroup, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("empty podcast query")
	}

	entries := ScanPodcastDirs(podcastsDir)
	if len(entries) == 0 {
		return nil, fmt.Errorf("no podcasts found in %s", podcastsDir)
	}

	if kind, ok := parsePodcastGroupKind(q); ok {
		members, label, err := filterGroup(entries, kind, isLocalEntryFavorite)
		if err != nil {
			return nil, err
		}
		return &ResolvedPodcastGroup{Kind: kind, Label: label, Entries: members}, nil
	}

	matched, err := MatchLocalPodcasts(entries, q)
	if err != nil {
		return nil, err
	}
	return &ResolvedPodcastGroup{
		Kind:    GroupKindSingle,
		Label:   matched.Title,
		Entries: []PodcastDirEntry{*matched},
	}, nil
}

// groupLabels are the display labels for each group kind. Local and backend
// group resolution render the same words; they differed only by element type.
var groupLabels = map[PodcastGroupKind]string{
	GroupKindAll:          "all podcasts",
	GroupKindFavorites:    "favorite podcasts",
	GroupKindNonFavorites: "non-favorite podcasts",
}

// filterGroup selects the members of a group kind from items, using isFavorite
// to decide membership for the favorite and non-favorite kinds.
func filterGroup[T any](items []T, kind PodcastGroupKind, isFavorite func(T) bool) ([]T, string, error) {
	label, ok := groupLabels[kind]
	if !ok {
		return nil, "", fmt.Errorf("unknown group kind %q", kind)
	}
	if kind == GroupKindAll {
		return items, label, nil
	}
	want := kind == GroupKindFavorites
	var out []T
	for _, it := range items {
		if isFavorite(it) == want {
			out = append(out, it)
		}
	}
	return out, label, nil
}

func isLocalEntryFavorite(e PodcastDirEntry) bool {
	return config.LoadPodcastConfig(e.Dir, config.PodcastConfig{}).Favorite
}

func isBackendPodcastFavorite(p backend.Podcast, podcastsDir string) bool {
	dir := p.Path
	if dir == "" {
		dir = findPodcastDirForItem(p, podcastsDir)
	}
	if dir == "" {
		return false
	}
	pCfg := config.LoadPodcastConfig(dir, config.PodcastConfig{})
	return pCfg.Favorite
}

type ResolvedBackendGroup struct {
	Kind     PodcastGroupKind
	Label    string
	Podcasts []backend.Podcast
}

func resolveBackendPodcastGroup(podcasts []backend.Podcast, podcastsDir, query string) (*ResolvedBackendGroup, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("empty podcast query")
	}

	if kind, ok := parsePodcastGroupKind(q); ok {
		members, label, err := filterGroup(podcasts, kind, func(p backend.Podcast) bool {
			return isBackendPodcastFavorite(p, podcastsDir)
		})
		if err != nil {
			return nil, err
		}
		return &ResolvedBackendGroup{Kind: kind, Label: label, Podcasts: members}, nil
	}

	matched, err := MatchBackendPodcasts(podcasts, q)
	if err != nil {
		return nil, err
	}
	return &ResolvedBackendGroup{
		Kind:     GroupKindSingle,
		Label:    backendPodcastTitle(*matched),
		Podcasts: []backend.Podcast{*matched},
	}, nil
}

// withFeedURL keeps only the podcasts that have an upstream feed, falling back
// to the whole list when none does. A podcast with no feed cannot be checked
// for new episodes, so narrowing to the ones that can is almost always what a
// command means — but returning nothing at all would be worse than returning
// everything.
func withFeedURL(podcasts []backend.Podcast) []backend.Podcast {
	var active []backend.Podcast
	for _, p := range podcasts {
		if strings.TrimSpace(p.Media.Metadata.FeedURL) != "" {
			active = append(active, p)
		}
	}
	if len(active) > 0 {
		return active
	}
	return podcasts
}

// SelectBackendTargets narrows a backend's podcast list to what a query names.
// An empty query selects everything that has a feed. A query naming a single
// podcast selects exactly that one, feed or not, because the user asked for it
// by name.
func (l *Library) SelectBackendTargets(podcasts []backend.Podcast, target string) ([]backend.Podcast, error) {
	if target == "" {
		return withFeedURL(podcasts), nil
	}
	group, err := resolveBackendPodcastGroup(podcasts, l.cfg.PodcastsDir, target)
	if err != nil {
		return nil, err
	}
	if group.Kind == GroupKindSingle {
		return group.Podcasts, nil
	}
	return withFeedURL(group.Podcasts), nil
}
