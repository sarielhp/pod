package backend

import (
	"strings"
	"unicode"
)

func sanitizePodcastName(s string) string {
	res := SanitizePodcastTitle(s)
	if res == "untitled" {
		return "podcast_escaped"
	}
	return res
}

func SanitizePodcastTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "untitled"
	}
	for _, q := range []string{"'", "\"", "`", "’", "‘", "“", "”"} {
		title = strings.ReplaceAll(title, q, "")
	}

	var sb strings.Builder
	lastUnderscore := false

	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
			lastUnderscore = false
		} else {
			if !lastUnderscore && sb.Len() > 0 {
				sb.WriteRune('_')
				lastUnderscore = true
			}
		}
	}

	res := strings.Trim(sb.String(), "_")
	if res == "" || res == ".." || res == "." {
		return "untitled"
	}
	runes := []rune(res)
	if len(runes) > 120 {
		res = strings.Trim(string(runes[:120]), "_")
	}
	if res == "" {
		return "untitled"
	}
	return res
}

func StripHTML(s string) string {
	var result strings.Builder
	inTag := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '<':
			inTag = true
		case c == '>':
			inTag = false
		case inTag:
			// Inside a tag: drop the byte.
		case c == '&':
			decoded, width := decodeEntity(s[i:])
			result.WriteString(decoded)
			i += width - 1
		default:
			result.WriteByte(c)
		}
	}
	return strings.TrimSpace(result.String())
}

// decodeEntity reads one HTML entity from the start of s, returning its
// replacement and how many bytes it consumed. An unterminated or unknown
// entity is passed through unchanged, so text that merely contains an
// ampersand survives intact.
func decodeEntity(s string) (string, int) {
	end := strings.IndexByte(s, ';')
	if end < 0 {
		return "&", 1
	}
	entity := s[:end+1]
	replacements := map[string]string{
		"&amp;":  "&",
		"&lt;":   "<",
		"&gt;":   ">",
		"&quot;": `"`,
		"&apos;": "'",
		"&nbsp;": " ",
	}
	if r, ok := replacements[entity]; ok {
		return r, len(entity)
	}
	return entity, len(entity)
}
