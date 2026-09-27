package pipeline

import (
	"pod/pkg/episode"
)

// QueueFileName is the per-podcast ad-removal queue filename.
const QueueFileName = episode.QueueFileName

// EpisodeStateStore aliases the store interface in pkg/episode.
type EpisodeStateStore = episode.EpisodeStateStore

// FileEpisodeStateStore aliases the store implementation in pkg/episode.
type FileEpisodeStateStore = episode.FileEpisodeStateStore

var (
	// DefaultStateStore points to the default file-based episode state store.
	DefaultStateStore EpisodeStateStore = episode.DefaultStateStore

	StatusPathFor              = episode.StatusPathFor
	LoadEpisodeStatus          = episode.LoadEpisodeStatus
	SaveEpisodeStatus          = episode.SaveEpisodeStatus
	GetOrCreateEpisodeStatus   = episode.GetOrCreateEpisodeStatus
	UpdateEpisodeStatus        = episode.UpdateEpisodeStatus
	IsEpisodeClean             = episode.IsEpisodeClean
	IsEpisodeCompleted         = episode.IsEpisodeCompleted
	IsEpisodeInRemoteFlight    = episode.IsEpisodeInRemoteFlight
	EpisodeDurations           = episode.EpisodeDurations
	ResolveAudioFiles          = episode.ResolveAudioFiles
	UpdateQueue                = episode.UpdateQueue
	AddToQueue                 = episode.AddToQueue
	AddToQueueChecked          = episode.AddToQueueChecked
	RemoveFromQueue            = episode.RemoveFromQueue
	ReadQueue                  = episode.ReadQueue
	QueuedEpisodes             = episode.QueuedEpisodes
	IsQueueAudioPath           = episode.IsQueueAudioPath
	ResolveQueueAudioPath      = episode.ResolveQueueAudioPath
	RemoveQueuedAudio          = episode.RemoveQueuedAudio
	ClearQueuePriority         = episode.ClearQueuePriority
	SetPublicationSource       = episode.SetPublicationSource
	SourcePublicationTime      = episode.SourcePublicationTime
	NormalizeEpisodeTitle      = episode.NormalizeEpisodeTitle
	ParseABSEpisodePublishedAt = episode.ParseABSEpisodePublishedAt
)
