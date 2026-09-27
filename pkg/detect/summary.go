package detect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"pod/pkg/types"
)

// PodcastSummaryInput provides the podcast metadata needed to generate a summary.
type PodcastSummaryInput struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Author      string `json:"author,omitempty"`
	Description string `json:"description,omitempty"`
}

// PodcastSummaryOutput holds the AI-generated icon and summary.
type PodcastSummaryOutput struct {
	Icon    string `json:"icon"`
	Summary string `json:"summary"`
}

const PodcastSummarySystemPrompt = `You are an expert podcast directory assistant.
For each podcast in the list, analyze its title, author, and description, and generate:
1. "icon": Exactly one Unicode emoji that best represents the podcast's topic (e.g. 🧠 for science/health, ⚖️ for law/crime, 🗞️ for daily news, 💻 for programming/technology, 🔬 for science, ⚽ for sports, 💰 for finance/business, 🎭 for arts/culture/comedy, 📚 for history/literature, 🎙️ for general talk).
2. "summary": A concise 2 to 3 sentence summary capturing what the podcast is about, its key topics, and style.

Return ONLY a valid JSON object mapping each podcast "id" to its {"icon": "...", "summary": "..."} object.
Example:
{
  "huberman-lab": {
    "icon": "🧠",
    "summary": "Huberman Lab explores neuroscience, discussing how our brain and its connections with body organs control our perceptions, behaviors, and health. Hosted by Dr. Andrew Huberman, it translates complex science into practical daily protocols."
  }
}
Do NOT include markdown fences, backticks, or any text outside the JSON object.`

const summaryBatchSize = 10

// BatchSummarizePodcasts sends podcast metadata in batches to an LLM to generate
// a 2-3 line summary and 1-character/emoji icon for each podcast.
func BatchSummarizePodcasts(ctx context.Context, profile types.LLMProfile, pods []PodcastSummaryInput) (map[string]PodcastSummaryOutput, error) {
	if len(pods) == 0 {
		return make(map[string]PodcastSummaryOutput), nil
	}

	results := make(map[string]PodcastSummaryOutput, len(pods))
	for i := 0; i < len(pods); i += summaryBatchSize {
		end := i + summaryBatchSize
		if end > len(pods) {
			end = len(pods)
		}
		chunk := pods[i:end]
		chunkResults, err := summarizePodcastChunk(ctx, profile, chunk)
		if err != nil {
			return results, fmt.Errorf("failed to summarize podcast batch: %w", err)
		}
		for k, v := range chunkResults {
			results[k] = v
		}
	}
	return results, nil
}

func summarizePodcastChunk(ctx context.Context, profile types.LLMProfile, chunk []PodcastSummaryInput) (map[string]PodcastSummaryOutput, error) {
	payload, err := json.Marshal(chunk)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal podcast chunk: %w", err)
	}

	userPrompt := fmt.Sprintf("Generate an emoji icon and 2-3 sentence summary for each of the following podcasts:\n%s", string(payload))
	content, _, err := CallLLMChatContext(ctx, profile, PodcastSummarySystemPrompt, userPrompt, 2048, 60*time.Second, profile.APIKey)
	if err != nil {
		return nil, err
	}

	return parsePodcastSummaryResponse(content)
}

func parsePodcastSummaryResponse(content string) (map[string]PodcastSummaryOutput, error) {
	clean := strings.TrimSpace(content)
	if strings.HasPrefix(clean, "```") {
		lines := strings.Split(clean, "\n")
		if len(lines) >= 2 && strings.HasPrefix(lines[0], "```") {
			lines = lines[1:]
		}
		if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
			lines = lines[:len(lines)-1]
		}
		clean = strings.TrimSpace(strings.Join(lines, "\n"))
	}

	var raw map[string]PodcastSummaryOutput
	if err := json.Unmarshal([]byte(clean), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse AI summary response as JSON: %w (content: %s)", err, truncate(clean, 100))
	}

	sanitized := make(map[string]PodcastSummaryOutput, len(raw))
	for id, out := range raw {
		icon := strings.TrimSpace(out.Icon)
		if icon == "" {
			icon = "🎙️"
		}
		summary := strings.TrimSpace(out.Summary)
		sanitized[id] = PodcastSummaryOutput{
			Icon:    icon,
			Summary: summary,
		}
	}
	return sanitized, nil
}
