package gemini

import (
	"context"
	"fmt"

	"pod/pkg/port"
	"pod/pkg/types"
	"pod/pkg/util"
)

// DefaultGeminiModelChain is re-exported for callers already speaking this
// package's vocabulary. The list itself lives in pkg/types so ad detection,
// which shares the quota but not this package, can see the same ordering.
var DefaultGeminiModelChain = types.DefaultGeminiModelChain

// GeminiModelChain is the list of models to try, in order, for this config.
//
// An explicitly configured model leads, because a stated preference should be
// honoured, but it does not stand alone: the point of the chain is that an
// exhausted or retired model is survivable, and that applies just as much to
// one the user named.
func GeminiModelChain(cfg *types.Config) []string {
	var chain []string
	seen := map[string]bool{}
	add := func(m string) {
		if m == "" || seen[m] {
			return
		}
		seen[m] = true
		chain = append(chain, m)
	}
	if cfg != nil {
		add(cfg.GeminiModel)
	}
	for _, m := range DefaultGeminiModelChain {
		add(m)
	}
	if len(chain) == 0 {
		add(defaultGeminiModel)
	}
	return chain
}

// callStudioAcrossModels transcribes one chunk, letting the port walk the
// model chain when a model reports itself unavailable.
//
// The port owns the retrying, the switching and the cooldown; this function
// owns only the protocol. What crosses between them is a verdict, never a
// response body, which is what lets ad detection share the same meter while
// speaking a completely different wire format to the same host.
func callStudioAcrossModels(ctx context.Context, apiKey, fileURI, mimeType string, models []string) (*types.GeminiResponsePayload, error) {
	p := StudioPort()
	p.Notify = announceModelSwitch

	var payload *types.GeminiResponsePayload
	err := p.Call(ctx, models, func(ctx context.Context, model string) port.Attempt {
		got, attempt := callGeminiStudioOnce(ctx, apiKey, model, fileURI, mimeType)
		if attempt.Verdict == port.Success {
			payload = got
		}
		return attempt
	})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func announceModelSwitch(from, to, reason string) {
	fmt.Println(util.BoldYellow(fmt.Sprintf("   Gemini: %s is unavailable (%s); trying %s", from, reason, to)))
}
