package types

import "strings"

// ProcOptions is everything the processing engine needs to know about a run:
// how loud to be, what to force, where to send the work. It is deliberately
// smaller than CLIOptions — the engine packages (adremoval, pipeline, detect)
// take this, so a flag that only the command line cares about cannot reach
// them, and adding one cannot change an engine signature's meaning.
//
// CLIOptions embeds it, so the flag parser binds straight into these fields and
// pkg/cli reads them by promotion. Callers that are not the command line —
// the offload worker, the remote scanner — build a ProcOptions directly
// instead of fabricating a command line that was never typed.
type ProcOptions struct {
	// Verbosity. Quiet suppresses progress reporting; Verbose adds detail.
	Quiet   bool
	Verbose bool

	// DryRun reports what a run would do without doing it. Count caps how many
	// episodes a run touches; zero means no cap.
	DryRun bool
	Count  int

	// Force selection. Force is the raw --force value; Normalize expands it
	// into the two booleans, so the engine never reads the string itself.
	Force           string
	ForceLLM        bool
	ForceTranscribe bool
	Recut           bool
	// RecutBoilerplate makes a recut also cut the podcast's recorded boilerplate,
	// matched against the episode's transcript. No model is called.
	RecutBoilerplate bool

	// Transcription. KeepAudio preserves the 16kHz mono audio extracted for
	// the engine, which is otherwise discarded with the rest of the working
	// files — useful when the source was a video and the audio is wanted too.
	KeepAudio      bool
	TranscribeMin  string
	UseChunks      bool
	WhisperEngine  string
	WhisperModel   string
	SaveTranscript bool
	TranscriptPath string

	// Ad detection.
	UseLLM string

	// Output and export.
	Output    string
	ExportSRT bool
	ExportTXT bool

	// Priority orders the ad-removal queue; higher runs first.
	Priority int

	// Transcript audit thresholds.
	AuditMinRatioStr string
	AuditMinChars    int
}

// Normalize expands the raw --force value into the individual force flags. The
// engine entry points call it themselves so a caller cannot hand them options
// that were never normalized.
func (o *ProcOptions) Normalize() {
	if o.Force == "" {
		return
	}
	f := strings.ToLower(o.Force)
	if f == "all" || strings.Contains(f, "whisper") || strings.Contains(f, "transcribe") {
		o.ForceTranscribe = true
	}
	if f == "all" || strings.Contains(f, "llm") || strings.Contains(f, "ads") {
		o.ForceLLM = true
	}
}

// TranscribeOptions holds options specific to the transcription stage.
type TranscribeOptions struct {
	Quiet          bool
	Verbose        bool
	KeepAudio      bool
	TranscribeMin  string
	UseChunks      bool
	WhisperEngine  string
	WhisperModel   string
	SaveTranscript bool
	TranscriptPath string
	ExportSRT      bool
	ExportTXT      bool
}

// DetectOptions holds options specific to the ad detection stage.
type DetectOptions struct {
	Quiet          bool
	Verbose        bool
	UseLLM         string
	TranscriptPath string
}

// CutOptions holds options specific to the audio cutting stage.
type CutOptions struct {
	Quiet   bool
	Verbose bool
	Output  string
	Recut   bool
}

// TranscribeOptions projects ProcOptions into options for the transcription stage.
func (o ProcOptions) TranscribeOptions() TranscribeOptions {
	return TranscribeOptions{
		Quiet:          o.Quiet,
		Verbose:        o.Verbose,
		KeepAudio:      o.KeepAudio,
		TranscribeMin:  o.TranscribeMin,
		UseChunks:      o.UseChunks,
		WhisperEngine:  o.WhisperEngine,
		WhisperModel:   o.WhisperModel,
		SaveTranscript: o.SaveTranscript,
		TranscriptPath: o.TranscriptPath,
		ExportSRT:      o.ExportSRT,
		ExportTXT:      o.ExportTXT,
	}
}

// DetectOptions projects ProcOptions into options for the ad detection stage.
func (o ProcOptions) DetectOptions() DetectOptions {
	return DetectOptions{
		Quiet:          o.Quiet,
		Verbose:        o.Verbose,
		UseLLM:         o.UseLLM,
		TranscriptPath: o.TranscriptPath,
	}
}

// CutOptions projects ProcOptions into options for the cutting stage.
func (o ProcOptions) CutOptions() CutOptions {
	return CutOptions{
		Quiet:   o.Quiet,
		Verbose: o.Verbose,
		Output:  o.Output,
		Recut:   o.Recut,
	}
}
