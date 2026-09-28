package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"pod/pkg/gemini"
	"pod/pkg/player"
)

// reportModelChain compares the configured Gemini model chain against the
// models the key can actually reach.
//
// The chain is hand-written and rots silently in both directions: a model can
// be withdrawn, and a model can appear that would have served. Neither shows
// up until a run fails or quietly falls back, so this makes the drift
// visible on demand.
func reportModelChain(w io.Writer, cfg *Config) error {
	audit, err := gemini.AuditModelChain(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("check models: %w", err)
	}

	fmt.Fprintf(w, "\nGemini model chain (%d, in order):\n", len(audit.Chain))
	missing := map[string]bool{}
	for _, m := range audit.Missing {
		missing[m] = true
	}
	for i, m := range audit.Chain {
		mark := "ok"
		if missing[m] {
			mark = "RETIRED — the endpoint no longer offers it"
		}
		fmt.Fprintf(w, "  %d. %-26s %s\n", i+1, m, mark)
	}

	if len(audit.Unlisted) > 0 {
		fmt.Fprintf(w, "\nComparable models not in the chain (%d):\n", len(audit.Unlisted))
		for _, m := range audit.Unlisted {
			fmt.Fprintf(w, "  %s\n", m)
		}
		fmt.Fprintln(w, "\n  Each carries its own daily allowance, so adding one is extra")
		fmt.Fprintln(w, "  capacity as well as a possible upgrade. Two caveats: being listed is")
		fmt.Fprintln(w, "  not the same as being callable — a model withdrawn from new users")
		fmt.Fprintln(w, "  still appears here and answers 404 — and ordering models is a quality")
		fmt.Fprintln(w, "  judgement. Measure with `pod detect --repeat` before promoting one.")
	}

	if audit.Stale() {
		return fmt.Errorf("model chain names %s, which no longer exists", strings.Join(audit.Missing, ", "))
	}
	fmt.Fprintln(w, "\nEvery model in the chain is still offered.")
	return nil
}

func runCheckCommand(config Config, cli CLIOptions) error {
	switch {
	case cli.TestModels:
		return reportModelChain(outFor(cli), &config)
	case cli.TestGemini:
		return testGeminiAPI(outFor(cli), &config, cli.Quiet)
	case cli.TestKitty:
		testKittyImage(outFor(cli), cli.Args)
		return nil
	case cli.TestPlayer:
		return reportPlayerBackends(outFor(cli))
	}
	if !testWhisperServer(outFor(cli), config.WhisperURL, config.WhisperWakeCommand, cli.Quiet) {
		return fmt.Errorf("whisper test failed")
	}
	return nil
}

// reportPlayerBackends says which players are installed and which one
// 'pod player play' will pick, because the choice changes what playback can
// do and nothing else tells the user which one they got.
func reportPlayerBackends(w io.Writer) error {
	fmt.Fprintln(w, "Audio players (tried in this order):")
	for _, b := range player.Backends() {
		state := "not found"
		if b.Path != "" {
			state = b.Path
		}
		fmt.Fprintf(w, "  %-7s %-40s %s\n", b.Name, state, b.Note)
	}
	selected, err := player.SelectedBackend()
	if err != nil {
		fmt.Fprintln(w, "\nERROR: pod player play cannot work until one of them is installed.")
		return err
	}
	fmt.Fprintf(w, "\npod player play will use: %s\n", selected.Name)
	if selected.Name != "mpv" {
		fmt.Fprintln(w, "mpv is absent: seeking restarts the player and desktop media keys (MPRIS) are unavailable. Install mpv to get both.")
	}
	return nil
}
