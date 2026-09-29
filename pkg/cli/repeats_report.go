package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"pod/pkg/detect"
	"pod/pkg/format"
	"pod/pkg/pipeline"
)

func printRepeatResults(cli CLIOptions, results []RepeatEpisodeResult) {
	w := outFor(cli)
	if cli.RepeatsEpisode != "" {
		printEpisodeSpans(w, results, cli.RepeatsEpisode)
		return
	}
	var total detect.Score
	scored := 0
	var repeated, length float64
	for _, r := range results {
		printRepeatRow(w, r)
		repeated += r.RepeatedSec
		length += r.Duration
		if r.Score != nil {
			total = total.Add(*r.Score)
			scored++
		}
	}
	fmt.Fprintf(w, "\n%d episodes, %s repeated of %s (%.1f%%)\n",
		len(results), format.FormatClock(repeated), format.FormatClock(length), 100*repeated/max(length, 1))
	if scored > 0 {
		fmt.Fprintf(w, "against truth on %d labelled episode(s):", scored)
		printScore(w, &total)
	}
}

func printRepeatRow(w io.Writer, r RepeatEpisodeResult) {
	fmt.Fprintf(w, "%-52s %8s %8s %5.1f%% %3d spans", clip(r.Episode, 52),
		format.FormatClock(r.Duration), format.FormatClock(r.RepeatedSec),
		100*r.RepeatedSec/max(r.Duration, 1), len(r.Spans))
	if r.Score != nil {
		fmt.Fprintf(w, "   P %5.1f%%  R %5.1f%%", 100*r.Score.Precision, 100*r.Score.Recall)
	}
	fmt.Fprintln(w)
}

func printEpisodeSpans(w io.Writer, results []RepeatEpisodeResult, match string) {
	for _, r := range results {
		if !strings.Contains(r.Episode, match) {
			continue
		}
		fmt.Fprintf(w, "%s: %d span(s), %s repeated of %s\n", r.Episode, len(r.Spans),
			format.FormatClock(r.RepeatedSec), format.FormatClock(r.Duration))
		for _, s := range r.Spans {
			fmt.Fprintf(w, "  %8s  %4s  in %2d other  %s\n", s.StartHMS,
				format.FormatClock(s.End-s.Start), s.Episodes, clip(s.Text, 160))
		}
	}
}

// catalogEntry gathers the spans, across a show, that begin with the same words.
type catalogEntry struct {
	key       string
	episodes  map[string]bool
	recurs    int
	positions []float64
	seconds   []float64
	sample    string
}

func printRepeatCatalog(cli CLIOptions, show *repeatShow) error {
	results := repeatResults(show, cli.RepeatsMinEpisodes)
	entries := buildCatalog(results)
	if cli.JSON {
		return encodeJSON(cli, catalogJSON(entries))
	}
	w := outFor(cli)
	fmt.Fprintf(w, "%d distinct recurring texts across %d episodes\n\n", len(entries), len(results))
	for i, e := range entries {
		if i == 25 {
			fmt.Fprintf(w, "... and %d more\n", len(entries)-i)
			break
		}
		fmt.Fprintf(w, "%3d eps  %-5s %4s  %s\n", e.recurs, positionLabel(median(e.positions)),
			format.FormatClock(median(e.seconds)), clip(e.sample, 150))
	}
	return nil
}

func buildCatalog(results []RepeatEpisodeResult) []*catalogEntry {
	byKey := map[string]*catalogEntry{}
	for _, r := range results {
		for _, s := range r.Spans {
			key := leadingWords(s.Text, 8)
			e := byKey[key]
			if e == nil {
				e = &catalogEntry{key: key, episodes: map[string]bool{}, sample: s.Text}
				byKey[key] = e
			}
			e.episodes[r.Episode] = true
			e.recurs = max(e.recurs, s.Episodes+1)
			e.positions = append(e.positions, s.Start/max(r.Duration, 1))
			e.seconds = append(e.seconds, s.End-s.Start)
		}
	}
	entries := make([]*catalogEntry, 0, len(byKey))
	for _, e := range byKey {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].recurs != entries[j].recurs {
			return entries[i].recurs > entries[j].recurs
		}
		return entries[i].key < entries[j].key
	})
	return entries
}

func catalogJSON(entries []*catalogEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"episodes": e.recurs, "position": positionLabel(median(e.positions)),
			"seconds": median(e.seconds), "text": e.sample,
		})
	}
	return out
}

// printRepeatCurve answers how good repetition alone can get as the number of
// episodes compared grows, scored against every episode that has labelled truth.
func printRepeatCurve(cli CLIOptions, show *repeatShow) error {
	labelled := labelledEpisodes(show)
	if len(labelled) == 0 {
		return fmt.Errorf("no labelled episodes: run 'pod detect --save-truth' on some of them first")
	}
	w := outFor(cli)
	fmt.Fprintf(w, "%d labelled episode(s) of %d\n\n%9s  %7s  %9s  %11s  %8s\n",
		len(labelled), show.corpus.Len(), "partners", "recall", "precision", "wrongly cut", "missed")
	for _, n := range curveSizes(show.corpus.Len() - 1) {
		total := curveScore(show, labelled, n)
		fmt.Fprintf(w, "%9d  %6.1f%%  %8.1f%%  %11s  %8s\n", n, 100*total.Recall, 100*total.Precision,
			format.FormatClock(total.FalsePositiveSec), format.FormatClock(total.MissedSec))
	}
	return nil
}

func labelledEpisodes(show *repeatShow) map[int]pipeline.TruthFile {
	out := map[int]pipeline.TruthFile{}
	for id, path := range show.paths {
		if truth, ok, err := pipeline.LoadTruth(path); ok && err == nil {
			out[id] = truth
		}
	}
	return out
}

func curveSizes(others int) []int {
	var sizes []int
	for n := 1; n < others; n *= 2 {
		sizes = append(sizes, n)
	}
	return append(sizes, others)
}

func curveScore(show *repeatShow, labelled map[int]pipeline.TruthFile, n int) detect.Score {
	var total detect.Score
	for id, truth := range labelled {
		var partners []int
		if n < show.corpus.Len()-1 {
			partners = show.corpus.SamplePartners(id, n, 1)
		}
		found := detect.RepeatSegments(show.corpus.Repeats(id, partners))
		total = total.Add(detect.ScoreSegments(found, truth.Segments))
	}
	return total
}

func leadingWords(text string, n int) string {
	words := strings.Fields(text)
	return strings.Join(words[:min(n, len(words))], " ")
}

func positionLabel(frac float64) string {
	switch {
	case frac < 0.1:
		return "start"
	case frac > 0.85:
		return "end"
	}
	return "mid"
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	return sorted[len(sorted)/2]
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
