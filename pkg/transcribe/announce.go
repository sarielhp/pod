package transcribe

import (
	"net/url"
	"os"
	"pod/pkg/format"
	"pod/pkg/progress"
	"pod/pkg/types"
	"pod/pkg/util"
	"strings"
)

// terminalReporter bridges this package's quiet/verbose flags to a Reporter
// for the functions whose callers still pass the flags rather than one.
func terminalReporter(quiet, verbose bool) progress.Reporter {
	if quiet {
		return progress.Discard
	}
	return progress.Writer(os.Stdout, os.Stderr, verbose)
}

func AnnounceStart(duration float64, rep progress.Reporter) {
	r := progress.Or(rep)
	r.Infof("\n%s", util.BoldYellow("--transcribing---"))
	r.Infof("%s", util.BoldCyan("Episode length: "+format.FormatMinutes(duration)))
}

func AnnounceWhisperServer(endpoint string, engine types.WhisperEngine, container string, rep progress.Reporter) {
	var parts []string
	if engine == types.WhisperEngineDocker && container != "" {
		parts = append(parts, "container: "+container)
	}
	if u, err := url.Parse(endpoint); err == nil && u.Hostname() != "" {
		parts = append(parts, "host: "+u.Hostname())
	}
	parts = append(parts, "model: chosen by server")
	AnnounceUsing(engine, "("+strings.Join(parts, ", ")+")", rep)
}

// BackendLabel names the backend that actually performs the transcription.
// Both Whisper backends are whisper.cpp, so the distinction that matters to
// the user is where it runs: "docker" is the whisper.cpp HTTP server in a
// container, "program" is the whisper-cli binary invoked on this host.
func BackendLabel(engine types.WhisperEngine) string {
	switch engine {
	case types.WhisperEngineLocal:
		return "Whisper (program)"
	case types.WhisperEngineDocker:
		return "Whisper (docker)"
	case types.WhisperEngineRemote:
		return "Whisper (remote)"
	case types.WhisperEngineGemini:
		return "Gemini"
	case "":
		return "Whisper"
	default:
		return "Whisper (" + string(engine) + ")"
	}
}

// AnnounceUsing reports the single line identifying the running backend.
// detail is an already-formatted parenthesised suffix, or empty.
func AnnounceUsing(engine types.WhisperEngine, detail string, rep progress.Reporter) {
	line := "\n" + util.BoldGreen("Using: "+BackendLabel(engine))
	if detail != "" {
		line += " " + util.Cyan(detail)
	}
	progress.Or(rep).Infof("%s", line)
}

// StampBackend records on the transcript which backend and model produced
// it. Without this a saved transcript gives no way to tell whether it came
// from the Docker server, the local program, or Gemini.
func StampBackend(td *types.TranscriptionData, engine types.WhisperEngine, model string) {
	if td == nil {
		return
	}
	td.Backend = BackendLabel(engine)
	if model != "" {
		td.Model = model
	}
}
