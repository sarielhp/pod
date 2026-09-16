package gemini

import "testing"

func TestComparableModel(t *testing.T) {
	t.Parallel()
	// Candidates are models of the same class as the chain. Lite and preview
	// variants are a different class, and listing them would bury the one
	// entry that matters.
	for _, name := range []string{"gemini-3.9-flash", "gemini-4-flash"} {
		if !comparableModel(name) {
			t.Errorf("%q should be a candidate", name)
		}
	}
	for _, name := range []string{
		"gemini-3.1-flash-lite",
		"gemini-3-flash-preview",
		"gemini-flash-latest", // an alias names no particular model
		"gemini-omni-1.1-flash",
		"gemini-2.5-pro", // not a flash model at all
	} {
		if comparableModel(name) {
			t.Errorf("%q should not be offered as a candidate", name)
		}
	}
}

func TestChainAuditStale(t *testing.T) {
	t.Parallel()
	if (ChainAudit{Chain: []string{"a"}}).Stale() {
		t.Error("a chain with nothing missing reported as stale")
	}
	// A chain naming a model the endpoint no longer offers is the failure
	// this check exists to surface: it stays invisible until a run falls back.
	if !(ChainAudit{Chain: []string{"a"}, Missing: []string{"a"}}).Stale() {
		t.Error("a retired chain entry was not reported")
	}
}

func TestSupportsGeneration(t *testing.T) {
	t.Parallel()
	if !supportsGeneration([]string{"countTokens", "generateContent"}) {
		t.Error("a model offering generateContent was rejected")
	}
	// Embedding and other models appear in the same listing and cannot
	// transcribe or detect anything.
	if supportsGeneration([]string{"embedContent"}) {
		t.Error("a non-generating model was accepted")
	}
}
