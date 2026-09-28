package backend

import "pod/pkg/types"

func ParsePubDate(pubStr string) int64 {
	return types.ParsePubDate(pubStr)
}

func GetPubMS(ep FeedEpisode) int64 {
	return types.GetPubMS(ep)
}

func AnalyzePodcastFrequency(episodes []FeedEpisode) PodcastFrequencyInfo {
	return types.AnalyzePodcastFrequency(episodes)
}
