package detect

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"pod/pkg/types"
)

func TestParsePodcastSummaryResponse(t *testing.T) {
	t.Parallel()

	raw := `
	{
		"pod1": {
			"icon": "🧠",
			"summary": "A podcast about neuroscience and daily health protocols."
		},
		"pod2": {
			"icon": "",
			"summary": "A daily news overview."
		}
	}`

	res, err := parsePodcastSummaryResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res["pod1"].Icon != "🧠" {
		t.Errorf("pod1 icon = %q, want %q", res["pod1"].Icon, "🧠")
	}
	if res["pod1"].Summary != "A podcast about neuroscience and daily health protocols." {
		t.Errorf("pod1 summary mismatch: %s", res["pod1"].Summary)
	}
	if res["pod2"].Icon != "🎙️" {
		t.Errorf("pod2 icon fallback = %q, want %q", res["pod2"].Icon, "🎙️")
	}
}

func TestParsePodcastSummaryResponseMarkdownFence(t *testing.T) {
	t.Parallel()

	raw := "```json\n" + `{"pod1": {"icon": "💻", "summary": "Software engineering interviews."}}` + "\n```"
	res, err := parsePodcastSummaryResponse(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res["pod1"].Icon != "💻" {
		t.Errorf("expected 💻, got %q", res["pod1"].Icon)
	}
}

func TestBatchSummarizePodcasts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"choices": [{"message": {"content": "{\"show1\": {\"icon\": \"🧠\", \"summary\": \"Brain science.\"}}"}}]}`)
	}))
	defer srv.Close()

	profile := types.LLMProfile{
		ID:    1,
		Name:  "test-llm",
		URL:   srv.URL,
		Model: "mock-model",
	}

	pods := []PodcastSummaryInput{
		{ID: "show1", Title: "Brain Show", Description: "Science podcast"},
	}

	results, err := BatchSummarizePodcasts(context.Background(), profile, pods)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results["show1"].Icon != "🧠" {
		t.Errorf("expected 🧠, got %q", results["show1"].Icon)
	}
	if results["show1"].Summary != "Brain science." {
		t.Errorf("expected 'Brain science.', got %q", results["show1"].Summary)
	}
}
