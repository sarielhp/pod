package types

import (
	"io"
	"os"
)

type AdSegment struct {
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Reason string  `json:"reason,omitempty"`
}

type KeepSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type TranscriptionSegment struct {
	Start    float64             `json:"start"`
	End      float64             `json:"end"`
	Text     string              `json:"text"`
	Language string              `json:"language,omitempty"`
	Words    []TranscriptionWord `json:"words,omitempty"`
}

type TranscriptionWord struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Word  string  `json:"word"`
}

type TranscriptionData struct {
	Text     string                 `json:"text"`
	Segments []TranscriptionSegment `json:"segments"`
	Language string                 `json:"language,omitempty"`
	// Backend and Model record which engine produced this transcript, so a
	// saved transcript can be traced back to the backend that made it.
	Backend string `json:"whisper_backend,omitempty"`
	Model   string `json:"whisper_model,omitempty"`
}

type CutEntry struct {
	StartSec       float64 `json:"start_sec"`
	EndSec         float64 `json:"end_sec"`
	DurationSec    float64 `json:"duration_sec"`
	StartFormatted string  `json:"start_formatted"`
	EndFormatted   string  `json:"end_formatted"`
	Reason         string  `json:"reason,omitempty"`
}

type MergedCutInterval struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type CutsData struct {
	Version             int                 `json:"version"`
	Generator           string              `json:"generator"`
	LLMUsed             string              `json:"llm_used"`
	TargetFile          string              `json:"target_file"`
	OriginalDurationSec float64             `json:"original_duration_sec"`
	TotalCutDurationSec float64             `json:"total_cut_duration_sec"`
	CutIntervals        []CutEntry          `json:"cut_intervals"`
	MergedCutIntervals  []MergedCutInterval `json:"merged_cut_intervals"`
	KeepIntervals       []KeepSegment       `json:"keep_intervals"`
}

type CutsResult struct {
	CutsFile     string
	KeepSegments [][2]float64
	Changed      bool
}

type LLMProfile struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	URL    string `json:"url"`
	Model  string `json:"model"`
	APIKey string `json:"api_key"`

	// Temperature is the sampling temperature for this profile. A pointer so
	// that zero — the value that asks a model to be as deterministic as it
	// can — is distinguishable from "not configured".
	Temperature *float64 `json:"temperature,omitempty"`
}

// DefaultAdDetectionTemperature is the sampling temperature used when a
// profile sets none.
//
// Ad detection is an extraction task, not a creative one: there is a right
// answer in the transcript and no value in varying it. A non-zero temperature
// bought nothing and cost reproducibility — the same episode returned
// different cut boundaries between runs.
const DefaultAdDetectionTemperature = 0.0

// SamplingTemperature is the temperature to send for this profile.
func (p LLMProfile) SamplingTemperature() float64 {
	if p.Temperature != nil {
		return *p.Temperature
	}
	return DefaultAdDetectionTemperature
}

type WhisperEngine string

const (
	WhisperEngineLocal  WhisperEngine = "local"
	WhisperEngineDocker WhisperEngine = "docker"
	WhisperEngineRemote WhisperEngine = "remote"
	WhisperEngineGemini WhisperEngine = "gemini"
)

// DefaultGeminiChunkSec is the audio per Gemini request when nothing overrides
// it. See Config.GetGeminiChunkSec for why it is not larger.
const DefaultGeminiChunkSec = 900.0

// DefaultGeminiModelChain is the order in which Gemini models are tried.
//
// The first entry is pinned rather than an alias on purpose.
// "gemini-flash-latest" silently follows Google's newest flash model, and the
// newest model carries the smallest free-tier allowance, so the alias drifts
// onto whatever is most rate-limited without anyone changing a line of code.
// Pinning also means a saved transcript names a real model.
//
// The later entries exist because the free tier meters requests per model.
// When the first model's quota is spent the second one's is untouched, which
// turns a hard stop into spare capacity. They also cover the other way a
// pinned model fails: retirement. Google withdrew gemini-2.5-flash from new
// users, and a pinned model will eventually answer 404 the same way.
//
// The ordering is a quality judgement and cannot be derived from version
// numbers, which is why it is an explicit list rather than something
// discovered at runtime. It needs revisiting as models ship.
var DefaultGeminiModelChain = []string{
	"gemini-3.8-flash",
	"gemini-3.7-flash",
	"gemini-3.6-flash",
	"gemini-3.5-flash",
}

type WhisperProfile struct {
	ID              int           `json:"id"`
	Name            string        `json:"name"`
	URL             string        `json:"url,omitempty"`
	SpeedFactor     float64       `json:"speed_factor"`
	DockerContainer string        `json:"docker_container,omitempty"`
	Language        string        `json:"language,omitempty"`
	Languages       []string      `json:"languages,omitempty"`
	Prompt          string        `json:"prompt,omitempty"`
	WakeCommand     string        `json:"wake_command,omitempty"`
	Engine          WhisperEngine `json:"engine"`
	Model           string        `json:"model,omitempty"`
	CliBinary       string        `json:"cli_binary,omitempty"`
	Processors      int           `json:"processors,omitempty"`
	Threads         int           `json:"threads,omitempty"`
	Greedy          bool          `json:"greedy,omitempty"`
}

type CostInfo struct {
	Type     string `json:"type"`
	In1M     float64
	Out1M    float64
	CostStr  string `json:"cost_str"`
	Est1HStr string `json:"est_1h_str"`
}

type TUIColorConfig struct {
	Cyan     string `json:"cyan,omitempty"`
	Purple   string `json:"purple,omitempty"`
	Magenta  string `json:"magenta,omitempty"`
	Pink     string `json:"pink,omitempty"`
	Yellow   string `json:"yellow,omitempty"`
	Green    string `json:"green,omitempty"`
	Red      string `json:"red,omitempty"`
	Blue     string `json:"blue,omitempty"`
	Lavender string `json:"lavender,omitempty"`
	DarkBg   string `json:"dark_bg,omitempty"`
	CardBg   string `json:"card_bg,omitempty"`
	Border   string `json:"border,omitempty"`
	Subtext  string `json:"subtext,omitempty"`
	Dim      string `json:"dim,omitempty"`
}

// RemoteOptions holds the remote-execution flags the command line acts on
// itself. The flags the engine acts on — Remote, Local, RemoteHost,
// RemoteFFmpegHost, NoCollect, Priority — live in ProcOptions.

type PolicyOptions struct {
	AutoDownloadStr  string
	DownloadPolicy   string
	DownloadK        int
	AutoCleanupStr   string
	CleanupDays      int
	AdRemovalMode    string
	FavoriteStr      string
	PolicyAll        bool
	SetDefaultPolicy bool
	NonFavorites     bool
}

type BackendOptions struct {
	ABSURL        string
	ABSUser       string
	ABSPass       string
	SetABS        bool
	ABSToken      string
	SqliteDBPath  string
	ServerSubcmd  string
	ForceDelete   bool
	Refresh       bool
	DisableHourly bool
	OPMLSubcmd    string
	OPMLFile      string
	PodcastsOnly  bool
	EpisodesOnly  bool

	// FeedJobs caps how many podcast feeds are fetched concurrently when
	// checking for new episodes. Zero selects the default.
	FeedJobs int
}

type CLIOptions struct {
	ProcOptions
	PolicyOptions
	BackendOptions

	ConfigCmd         string
	ConfigKey         string
	ConfigVal         string
	SetDefault        int
	PodcastsDir       string
	SetPodcastsDir    bool
	ListLLMs          bool
	CopyOpenCode      bool
	Debug             bool
	TestWhisper       bool
	TestKitty         bool
	TestGemini        bool
	TestModels        bool
	ResetCache        bool
	AddWhisper        string
	RemoveWhisper     int
	SetDefaultWhisper int
	ListWhispers      bool

	CountGiven     bool
	Podcast        string
	Fill           bool
	DownloadAll    bool
	FavoriteOff    bool
	KeepCount      *int
	CheckNew       bool
	Oldest         bool
	NoWait         bool
	ProcessorCmd   string
	ProcessorValue string
	ConfigInfo     bool
	Args           []string
	ProcSubcmd     string
	ExportFormat   string
	ShowCuts       bool
	ShowTranscript bool
	LsSubcmd       string
	JSON           bool
	InfoSubcmd     string
	QueueSubcmd    string
	PlayerSubcmd   string
	SyncSubcmd     string
	StatusSubcmd   string
	Latest         bool
	ShowExamples   bool

	// DetectRaw and DetectWriteCuts belong to `pod detect`. Raw reports the
	// model's own intervals rather than the merged ones the cutter would use,
	// which is what you want when judging the model instead of the cut.
	DetectRaw       bool
	DetectWriteCuts bool
	DetectRepeat    int

	// DownloadedOnly restricts `info latest` to episodes on disk. The default
	// lists everything the feeds have published, which is the useful question
	// when most podcasts are configured not to download automatically.
	DownloadedOnly bool

	// Temperature overrides the sampling temperature for one run, so that the
	// effect of changing it can be measured with `pod detect --repeat`
	// rather than argued about.
	Temperature string

	// IncludeHourly keeps hourly news bulletins in `info latest`. They are
	// hidden by default because one of them can publish more episodes in a
	// week than the rest of the library combined.
	IncludeHourly bool

	// Out and Err are where this invocation's output goes. Both nil means the
	// process streams, which is what a real command line wants. Tests supply
	// buffers instead, so that checking what a command printed does not mean
	// reassigning os.Stdout — a process-wide mutation that made every such
	// test unable to run in parallel with any other.
	Out io.Writer
	Err io.Writer
}

type WhisperConfig struct {
	WhisperURL             string           `json:"whisper_url"`
	WhisperSpeedFactor     float64          `json:"whisper_speed_factor"`
	WhisperDockerContainer string           `json:"whisper_docker_container"`
	WhisperLanguage        string           `json:"whisper_language"`
	WhisperPrompt          string           `json:"whisper_prompt"`
	WhisperWakeCommand     string           `json:"whisper_wake_command,omitempty"`
	WhisperEngine          WhisperEngine    `json:"whisper_engine,omitempty"`
	WhisperModel           string           `json:"whisper_model,omitempty"`
	WhisperCliBinary       string           `json:"whisper_cli_binary,omitempty"`
	WhisperProcessors      int              `json:"whisper_processors,omitempty"`
	WhisperThreads         int              `json:"whisper_threads,omitempty"`
	WhisperGreedy          bool             `json:"whisper_greedy,omitempty"`
	ActiveWhisperID        int              `json:"active_whisper_id,omitempty"`
	WhisperProfiles        []WhisperProfile `json:"whisper_profiles,omitempty"`
}

type BackendConfig struct {
	BackendType    string `json:"backend_type,omitempty"`
	PodfetchURL    string `json:"podfetch_url,omitempty"`
	PodfetchUser   string `json:"podfetch_user,omitempty"`
	PodfetchPass   string `json:"podfetch_pass,omitempty"`
	PodfetchAPIKey string `json:"podfetch_api_key,omitempty"`
	PodfetchDBPath string `json:"podfetch_db_path,omitempty"`
}

type PolicyConfig struct {
	DefaultDownloadPolicy string `json:"default_download_policy,omitempty"`
	DefaultDownloadK      int    `json:"default_download_k,omitempty"`
	DefaultAdRemoval      string `json:"default_ad_policy,omitempty"`
}

type GeminiConfig struct {
	GeminiProjectID         string `json:"gemini_project_id,omitempty"`
	GeminiStagingBucket     string `json:"gemini_staging_bucket,omitempty"`
	GeminiLocation          string `json:"gemini_location,omitempty"`
	GeminiAPIKey            string `json:"gemini_api_key,omitempty"`
	GeminiAPIKeyFile        string `json:"gemini_api_key_file,omitempty"`
	GeminiModel             string `json:"gemini_model,omitempty"`
	GeminiChunkSec          int    `json:"gemini_chunk_sec,omitempty"`
	GeminiAPIKeyEnabled     *bool  `json:"gemini_api_key_enabled,omitempty"`
	OpenRouterAPIKeyEnabled *bool  `json:"openrouter_api_key_enabled,omitempty"`
}

type SpeculativeConfig struct {
	SpeculativeTranscription *bool    `json:"speculative_transcription,omitempty"`
	SpeculativeCompetition   *bool    `json:"speculative_competition,omitempty"`
	SpeculativeServices      []string `json:"speculative_services,omitempty"`
	CompetingServices        []string `json:"competing_services,omitempty"`
}

type Config struct {
	Instructions      string          `json:"_instructions"`
	PodcastsDir       string          `json:"podcasts_dir"`
	ServerBaseURL     string          `json:"server_base_url,omitempty"`
	SubscriptionsFile string          `json:"subscriptions_file,omitempty"`
	ChunkDurationSec  int             `json:"chunk_duration_sec"`
	ActiveProfileID   int             `json:"active_profile_id"`
	Profiles          []LLMProfile    `json:"profiles"`
	PostProcessors    []string        `json:"post_processors,omitempty"`
	TUIColor          *TUIColorConfig `json:"tui_color,omitempty"`

	WhisperConfig
	BackendConfig
	PolicyConfig
	GeminiConfig
	SpeculativeConfig
}

func (c *Config) IsGeminiAPIKeyEnabled() bool {
	if c != nil && c.GeminiAPIKeyEnabled != nil {
		return *c.GeminiAPIKeyEnabled
	}
	return true
}

func (c *Config) IsOpenRouterAPIKeyEnabled() bool {
	if c != nil && c.OpenRouterAPIKeyEnabled != nil {
		return *c.OpenRouterAPIKeyEnabled
	}
	return true
}

func (c *Config) IsSpeculativeTranscriptionEnabled() bool {
	if c == nil {
		return false
	}
	if c.SpeculativeCompetition != nil {
		return *c.SpeculativeCompetition
	}
	if c.SpeculativeTranscription != nil {
		return *c.SpeculativeTranscription
	}
	return false
}

func (c *Config) GetCompetingServices() []string {
	if c == nil {
		return []string{"gemini", "whisper"}
	}
	if len(c.CompetingServices) > 0 {
		return c.CompetingServices
	}
	if len(c.SpeculativeServices) > 0 {
		return c.SpeculativeServices
	}
	return []string{"gemini", "whisper"}
}

func (c *Config) GetGeminiModel() string {
	if c != nil && c.GeminiModel != "" {
		return c.GeminiModel
	}
	return "gemini-flash-latest"
}

// GetGeminiChunkSec is how much audio is sent to Gemini in one request.
//
// The ceiling is the length of the reply, not the upload: a chunk of 30
// minutes comes back as an empty candidate with blockReason "OTHER", because
// the verbatim transcript of it does not fit in one response. Twenty minutes
// answers reliably, so the default leaves a margin below that. Chunks are
// transcribed in parallel, so a smaller value is not slower.
func (c *Config) GetGeminiChunkSec() float64 {
	if c != nil && c.GeminiChunkSec > 0 {
		return float64(c.GeminiChunkSec)
	}
	return DefaultGeminiChunkSec
}

// GeminiChunkSecCapped honours chunk_duration_sec when it asks for chunks
// shorter than Gemini can answer, and ignores it otherwise. That setting is a
// whisper.cpp knob; letting a large value reach Gemini is exactly what makes
// it return an empty candidate, so it may shrink the chunk but never grow it.
func (c *Config) GeminiChunkSecCapped(whisperChunkSec int) float64 {
	limit := c.GetGeminiChunkSec()
	if whisperChunkSec > 0 && float64(whisperChunkSec) < limit {
		return float64(whisperChunkSec)
	}
	return limit
}

func (c *Config) GetGeminiProjectID() string {
	if c != nil && c.GeminiProjectID != "" {
		return c.GeminiProjectID
	}
	return os.Getenv("GEMINI_PROJECT_ID")
}

func (c *Config) GetGeminiStagingBucket() string {
	if c != nil && c.GeminiStagingBucket != "" {
		s := c.GeminiStagingBucket
		if len(s) >= 5 && s[:5] == "gs://" {
			return s[5:]
		}
		return s
	}
	s := os.Getenv("GEMINI_STAGING_BUCKET")
	if len(s) >= 5 && s[:5] == "gs://" {
		return s[5:]
	}
	return s
}

func (c *Config) GetGeminiLocation() string {
	if c != nil && c.GeminiLocation != "" {
		return c.GeminiLocation
	}
	return "us-central1"
}
