package episode

import (
	"regexp"
	"strings"
	"time"

	"pod/pkg/backend"
)

var guidSuffixRegex = regexp.MustCompile(`\s*\([0-9a-fA-F-]{6,}\)$`)

// NormalizeEpisodeTitle normalizes episode filenames or titles for fuzzy comparison.
func NormalizeEpisodeTitle(name string) string {
	base := strings.TrimSuffix(name, ".mp3")
	base = guidSuffixRegex.ReplaceAllString(base, "")
	return strings.ToLower(strings.TrimSpace(base))
}

// ParseABSEpisodePublishedAt parses publication timestamps from a backend episode.
func ParseABSEpisodePublishedAt(ep *backend.Episode) int64 {
	if ep == nil {
		return 0
	}
	if ep.PublishedAt > 0 {
		return ep.PublishedAt
	}
	if ep.PubDate != "" {
		formats := []string{
			time.RFC1123,
			time.RFC1123Z,
			time.RFC3339,
			time.RFC822,
			time.RFC822Z,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05.000Z",
			"2006-01-02",
		}
		for _, f := range formats {
			if t, err := time.Parse(f, strings.TrimSpace(ep.PubDate)); err == nil {
				return t.UnixMilli()
			}
		}
	}
	return 0
}
