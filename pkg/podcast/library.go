package podcast

import (
	"pod/pkg/backend"
	"pod/pkg/progress"
)

// Config is everything the podcast library needs to know about its
// surroundings. It is deliberately these three fields and not the
// application's 48-field types.Config: the library manages a directory of
// podcasts and publishes a feed for them, and it has no business seeing
// transcription profiles, LLM keys, or the terminal UI's colour scheme.
type Config struct {
	// PodcastsDir is the library root — the directory holding one folder per
	// podcast. Without it neither a podcast's own settings nor the audio
	// already on disk can be found.
	PodcastsDir string

	// SubscriptionsFile is where the subscription store lives. Empty selects
	// the default location for the current user.
	SubscriptionsFile string

	// ServerBaseURL is the public root the generated feed and pages are served
	// from. Empty renders them with document-relative URLs.
	ServerBaseURL string
}

// Library is the entry point to a podcast library: the podcasts on disk, the
// subscriptions behind them, the feed cache, and the download queue.
//
// It owns the state that used to be reached through package-level singletons,
// so callers hold a Library instead of reaching into the package. Pure helpers
// — filename formatting, feed parsing, ID generation — stay package-level
// functions; routing them through a receiver would only make them look stateful.
type Library struct {
	cfg       Config
	backend   backend.Backend
	feedCache *FeedCacheManager
	queue     *DownloadQueue
	progress  progress.Reporter
}

// Open returns a Library over cfg. A nil Reporter is silent.
//
// The feed cache and download queue are the process-wide instances for their
// default paths. They are shared rather than per-Library on purpose: each
// serialises access to one file through its own mutex, so two instances over
// the same path would serialise against different mutexes and race on it.
func Open(cfg Config, b backend.Backend, rep progress.Reporter) *Library {
	return &Library{
		cfg:       cfg,
		backend:   b,
		feedCache: defaultFeedCache(),
		queue:     defaultDownloadQueue(),
		progress:  progress.Or(rep),
	}
}

func (l *Library) Config() Config               { return l.cfg }
func (l *Library) Backend() backend.Backend     { return l.backend }
func (l *Library) FeedCache() *FeedCacheManager { return l.feedCache }
func (l *Library) Queue() *DownloadQueue        { return l.queue }
func (l *Library) Progress() progress.Reporter  { return l.progress }

// Podcasts lists the podcast directories under the library root, assigning a
// short ID to any that lacks one.
func (l *Library) Podcasts() []PodcastDirEntry {
	return ScanPodcastDirs(l.cfg.PodcastsDir)
}

// Resolve turns a user's query into whatever it names — a podcast or a single
// episode — by short ID, folder name, title, or list position.
func (l *Library) Resolve(query string) (*ResolvedID, error) {
	return ResolveAnyID(l.cfg.PodcastsDir, query)
}

// ResolveGroup turns a query into a set of podcasts, so that "all", "fav" and
// a single podcast's name are all answerable the same way.
func (l *Library) ResolveGroup(query string) (*ResolvedPodcastGroup, error) {
	return ResolvePodcastGroup(l.cfg.PodcastsDir, query)
}

// Subscriptions opens the subscription store for this library.
func (l *Library) Subscriptions() (*SubscriptionStore, error) {
	return NewSubscriptionStore(l.cfg.SubscriptionsFile)
}

// PodcastDir returns the directory a subscription's episodes live in.
func (l *Library) PodcastDir(sub Subscription) string {
	return resolvePodcastDirForSub(sub, l.cfg.PodcastsDir)
}

// Publish regenerates one podcast's feed.xml and index.html.
func (l *Library) Publish(sub Subscription, feedEpisodes []backend.FeedEpisode) error {
	return PublishPodcast(l.PodcastDir(sub), sub, l.cfg.ServerBaseURL, feedEpisodes)
}

// PublishCatalog regenerates the library index page across all podcasts.
func (l *Library) PublishCatalog(subs []Subscription) error {
	return PublishCatalog(l.cfg.PodcastsDir, subs)
}

// CleanOrphans removes podcast entries whose audio has disappeared.
func (l *Library) CleanOrphans(opts CleanOrphansOptions) (CleanOrphansResult, error) {
	return RunCleanOrphans(l.backend, opts)
}
