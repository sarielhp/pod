// Package cli implements the command-line interface and commands for pod.
package cli

import (
	"fmt"
	"io"
	"os"

	"pod/pkg/backend"
	"pod/pkg/config"
	"pod/pkg/podcast"
	"pod/pkg/progress"
	"pod/pkg/types"
	"pod/pkg/util"
)

type (
	// Config aliases types.Config for package cli.
	Config = types.Config
	// CLIOptions aliases types.CLIOptions for package cli.
	CLIOptions = types.CLIOptions
	// ProcOptions aliases types.ProcOptions for package cli.
	ProcOptions = types.ProcOptions
	// PolicyOptions aliases types.PolicyOptions for package cli.
	PolicyOptions = types.PolicyOptions
	// LLMProfile aliases types.LLMProfile for package cli.
	LLMProfile = types.LLMProfile
	// AdSegment aliases types.AdSegment for package cli.
	AdSegment = types.AdSegment
	// TranscriptionData aliases types.TranscriptionData for package cli.
	TranscriptionData = types.TranscriptionData
	// CutsData aliases types.CutsData for package cli.
	CutsData = types.CutsData
	// CutEntry aliases types.CutEntry for package cli.
	CutEntry = types.CutEntry
	// EpisodeStatusFile aliases types.EpisodeStatusFile for package cli.
	EpisodeStatusFile = types.EpisodeStatusFile
	// PlayerTrack aliases types.PlayerTrack for package cli.
	PlayerTrack = types.PlayerTrack
	// Podcast aliases backend.Podcast for package cli.
	Podcast = backend.Podcast
	// PodcastConfig aliases config.PodcastConfig for package cli.
	PodcastConfig = config.PodcastConfig
	// FeedEpisode aliases backend.FeedEpisode for package cli.
	FeedEpisode = backend.FeedEpisode
	// Episode aliases backend.Episode for package cli.
	Episode = backend.Episode
	// WhisperEngine aliases types.WhisperEngine for package cli.
	WhisperEngine = types.WhisperEngine
	// WhisperProfile aliases types.WhisperProfile for package cli.
	WhisperProfile = types.WhisperProfile
	// ResolvedPodcast aliases podcast.ResolvedPodcast for package cli.
	ResolvedPodcast = podcast.ResolvedPodcast
	// ResolvedEpisode aliases podcast.ResolvedEpisode for package cli.
	ResolvedEpisode = podcast.ResolvedEpisode
	// ResolvedID aliases podcast.ResolvedID for package cli.
	ResolvedID = podcast.ResolvedID
	syncWG     = util.SyncWG
)

// Aliased policy and state constants imported across cli subcommands.
const (
	AdRemovalNone   = config.AdRemovalNone
	AdRemovalLatest = config.AdRemovalLatest
	AdRemovalAll    = config.AdRemovalAll

	DownloadPolicyNone    = config.DownloadPolicyNone
	DownloadPolicyLatest  = config.DownloadPolicyLatest
	DownloadPolicyLatestK = config.DownloadPolicyLatestK
	DownloadPolicyAll     = config.DownloadPolicyAll

	StateTranscribingLocally  = types.StateTranscribingLocally
	StateTranscribingRemotely = types.StateTranscribingRemotely
	StateCuttingLocally       = types.StateCuttingLocally
	StateCuttingRemotely      = types.StateCuttingRemotely
	StateQueuedRemote         = types.StateQueuedRemote
	StateCopiedBack           = types.StateCopiedBack
	StateDone                 = types.StateDone
	StateDownloaded           = types.StateDownloaded

	BatchStatusProcessing = types.BatchStatusProcessing
	BatchStatusCompleted  = types.BatchStatusCompleted
	BatchStatusFailed     = types.BatchStatusFailed

	WhisperEngineLocal  = types.WhisperEngineLocal
	WhisperEngineDocker = types.WhisperEngineDocker
)

func fatalError(formatStr string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, formatStr, args...)
	os.Exit(1)
}

func loadConfig() Config {
	cfg, err := config.LoadConfig()
	if err != nil || cfg == nil {
		c := config.DefaultConfig()
		return c
	}
	podcast.SetMaxEpisodeBytes(int64(cfg.MaxEpisodeMB) << 20)
	podcast.SetAllowPrivateHosts(cfg.AllowPrivateHosts)
	return *cfg
}

// reporter turns the CLI's quiet/verbose flags into the progress.Reporter that
// library calls take. Quiet discards; otherwise info goes to stdout, warnings
// to stderr, and detail only when --verbose.
func reporter(cli CLIOptions) progress.Reporter {
	if cli.Quiet {
		return progress.Discard
	}
	return progress.Writer(os.Stdout, os.Stderr, cli.Verbose)
}

// library opens the podcast library for this command. It is the CLI's single
// translation point from the 48-field application config to the three fields
// the library actually needs.
func library(cfg Config, cli CLIOptions, b backend.Backend) *podcast.Library {
	podcastsDir := cfg.PodcastsDir
	if cli.PodcastsDir != "" {
		podcastsDir = cli.PodcastsDir
	}
	return podcast.Open(podcast.Config{
		PodcastsDir:       podcastsDir,
		SubscriptionsFile: config.SubscriptionsFilePath(&cfg),
		ServerBaseURL:     cfg.ServerBaseURL,
	}, b, reporter(cli))
}

// outFor is where a command's results go: the caller's writer if it supplied
// one, otherwise stdout.
//
// It deliberately does NOT consult --quiet. For most commands quiet means "no
// progress chatter", but for `info ls --quiet` it means "print bare IDs and
// nothing else" — output that must survive. Conflating the two silently
// emptied that command.
// inFor is where a command reads a confirmation from: the caller's reader if
// it supplied one, otherwise stdin.
func inFor(cli CLIOptions) io.Reader {
	if cli.In != nil {
		return cli.In
	}
	return os.Stdin
}

func outFor(cli CLIOptions) io.Writer {
	if cli.Out != nil {
		return cli.Out
	}
	return os.Stdout
}

// progressFor is the stream for progress and status chatter — the output that
// --quiet exists to suppress. Printing through it makes that a property of the
// writer rather than something each call site must remember, which is how
// `server download --dry-run --quiet` came to print 11 KB.
func progressFor(cli CLIOptions) io.Writer {
	if cli.Quiet {
		return io.Discard
	}
	return outFor(cli)
}

// errFor is the stream for warnings and errors. Unlike outFor it ignores
// --quiet: suppressing progress is not the same as hiding a problem.
func errFor(cli CLIOptions) io.Writer {
	if cli.Err != nil {
		return cli.Err
	}
	return os.Stderr
}
