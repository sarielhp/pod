package types

import (
	"sort"
	"time"
)

type PodcastCadence string

const (
	CadenceHourly       PodcastCadence = "hourly"
	CadenceDaily        PodcastCadence = "daily"
	CadenceWeekly       PodcastCadence = "weekly"
	CadenceMonthly      PodcastCadence = "monthly"
	CadenceIntermittent PodcastCadence = "intermittent"
)

func ParsePubDate(pubStr string) int64 {
	if pubStr == "" {
		return 0
	}
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		time.RFC3339Nano,
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 MST",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
		"02 Jan 2006 15:04:05 -0700",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, pubStr); err == nil {
			return t.UnixNano() / 1e6
		}
	}
	return 0
}

func GetPubMS(ep FeedEpisode) int64 {
	if ep.PublishedAt > 0 {
		return ep.PublishedAt
	}
	if ep.PubDate != "" {
		return ParsePubDate(ep.PubDate)
	}
	return 0
}

func AnalyzePodcastFrequency(episodes []FeedEpisode) PodcastFrequencyInfo {
	if len(episodes) < 3 {
		return PodcastFrequencyInfo{
			Type:             string(CadenceIntermittent),
			EpisodesAnalyzed: len(episodes),
			AnalyzedAt:       time.Now().UTC(),
		}
	}

	var timestamps []int64
	for _, ep := range episodes {
		if ms := GetPubMS(ep); ms > 0 {
			timestamps = append(timestamps, ms)
		}
	}

	if len(timestamps) < 3 {
		return PodcastFrequencyInfo{
			Type:             string(CadenceIntermittent),
			EpisodesAnalyzed: len(episodes),
			AnalyzedAt:       time.Now().UTC(),
		}
	}

	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})

	intervals := make([]float64, len(timestamps)-1)
	for i := 0; i < len(timestamps)-1; i++ {
		diffMs := max(0, timestamps[i+1]-timestamps[i])
		intervals[i] = float64(diffMs) / (1000.0 * 3600.0)
	}

	spanDays := (float64(timestamps[len(timestamps)-1]-timestamps[0]) / (1000.0 * 3600.0)) / 24.0
	avgDaysInterval := spanDays / float64(len(intervals))

	var releasesPerDay, episodesPerWeek float64
	if avgDaysInterval > 0 {
		releasesPerDay = 1.0 / avgDaysInterval
		episodesPerWeek = releasesPerDay * 7.0
	}

	sortedIntervals := make([]float64, len(intervals))
	copy(sortedIntervals, intervals)
	sort.Float64s(sortedIntervals)

	var medianHoursInterval float64
	n := len(sortedIntervals)
	if n%2 == 1 {
		medianHoursInterval = sortedIntervals[n/2]
	} else {
		medianHoursInterval = (sortedIntervals[n/2-1] + sortedIntervals[n/2]) / 2.0
	}

	var cadence PodcastCadence
	if releasesPerDay > 10.0 || medianHoursInterval <= 4.0 {
		cadence = CadenceHourly
	} else if len(episodes) < 10 {
		cadence = CadenceIntermittent
	} else if episodesPerWeek >= 4.0 || medianHoursInterval <= 48.0 {
		cadence = CadenceDaily
	} else if episodesPerWeek >= 0.75 || medianHoursInterval <= 240.0 {
		cadence = CadenceWeekly
	} else if medianHoursInterval <= 1080.0 {
		cadence = CadenceMonthly
	} else {
		cadence = CadenceIntermittent
	}

	return PodcastFrequencyInfo{
		Type:                string(cadence),
		EpisodesAnalyzed:    len(episodes),
		AvgDaysInterval:     avgDaysInterval,
		MedianHoursInterval: medianHoursInterval,
		EpisodesPerWeek:     episodesPerWeek,
		AnalyzedAt:          time.Now().UTC(),
	}
}
