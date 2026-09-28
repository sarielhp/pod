package backend

import "pod/pkg/types"

func sanitizePodcastName(s string) string {
	res := SanitizePodcastTitle(s)
	if res == "untitled" {
		return "podcast_escaped"
	}
	return res
}

func SanitizePodcastTitle(title string) string {
	return types.SanitizePodcastTitle(title)
}

func StripHTML(s string) string {
	return types.StripHTML(s)
}
