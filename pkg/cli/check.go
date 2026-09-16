package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"pod/pkg/gemini"
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
		fmt.Fprintln(w, "  capacity as well as a possible upgrade. Ordering them is a quality")
		fmt.Fprintln(w, "  judgement — measure with `pod detect --repeat` before promoting one.")
	}

	if audit.Stale() {
		return fmt.Errorf("model chain names %s, which no longer exists", strings.Join(audit.Missing, ", "))
	}
	fmt.Fprintln(w, "\nEvery model in the chain is still offered.")
	return nil
}

func runCheckCommand(config Config, cli CLIOptions) error {
	if cli.TestModels {
		return reportModelChain(outFor(cli), &config)
	}
	if cli.TestKitty {
		testKittyImage(outFor(cli), cli.Args)
	} else if cli.TestGemini {
		return testGeminiAPI(outFor(cli), &config, cli.Quiet)
	} else {
		if !testWhisperServer(outFor(cli), config.WhisperURL, config.WhisperWakeCommand, cli.Quiet) {
			return fmt.Errorf("whisper test failed")
		}
	}
	return nil
}
