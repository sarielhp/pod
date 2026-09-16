package types

const GeminiAdRemovalPrompt = `You are an expert audio editor analyzing a podcast episode.
Your task is to:
1. Identify all non-content intervals to be removed:
   - "advertisement": commercial breaks, sponsor plugs, promotional host-reads (including Hebrew: חסויות, קודי קופון, שיתופי פעולה).
   - "music_interlude": extended transition or filler music without speech longer than 5 seconds.
   - "intro_outro": pre-roll or post-roll theme songs and disclaimers.
2. Provide a verbatim timestamped transcript for the remaining spoken content.

Return ONLY a valid JSON object strictly matching this schema:
{
  "cuts": [
    {"start": 12.5, "end": 45.0, "type": "advertisement", "reason": "Sponsor plug for Wolt"}
  ],
  "segments": [
    {"start": 45.0, "end": 52.3, "text": "ברוכים הבאים לפרק..."}
  ]
}`

type GeminiCutItem struct {
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Type   string  `json:"type"`
	Reason string  `json:"reason"`
}

type GeminiSegmentItem struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type GeminiResponsePayload struct {
	Cuts     []GeminiCutItem     `json:"cuts"`
	Segments []GeminiSegmentItem `json:"segments"`

	// ModelVersion is the model the API reports as having answered, which is
	// not necessarily the one that was requested: an alias such as
	// "gemini-flash-latest" resolves to whatever is current. It is carried so
	// the transcript can record what actually produced it.
	ModelVersion string `json:"-"`
}

type GeminiChunkInfo struct {
	Index    int
	StartSec float64
	DurSec   float64
	FilePath string
}

type GeminiChunkResult struct {
	Index    int
	StartSec float64
	Payload  *GeminiResponsePayload
}
