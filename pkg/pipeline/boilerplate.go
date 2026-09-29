package pipeline

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"pod/pkg/config"
	"pod/pkg/detect"
	"pod/pkg/types"
)

// DefaultBoilerplateEpisodes is how many episodes must carry a text before it
// counts as boilerplate: more than five. Real content that legitimately recurs,
// such as a series revisiting a passage, shows up in two or three episodes;
// standing intros, credits and sponsor reads show up in dozens.
const DefaultBoilerplateEpisodes = 6

// MinAnalyzeTranscripts is the fewest transcripts worth analysing. Below it a
// text cannot reach the threshold, and "recurs" would mean little.
const MinAnalyzeTranscripts = 10

// AnalyzeOptions tunes the boilerplate analysis of a podcast.
type AnalyzeOptions struct {
	MinEpisodes int
	MinWords    int
	DryRun      bool
}

// AnalyzeResult reports what an analysis found and changed.
type AnalyzeResult struct {
	Dir         string
	Transcripts int
	Phrases     []config.BoilerplatePhrase
	Added       int
	Dropped     int
	// Skipped explains why nothing was analysed, when that is so.
	Skipped string
}

// AnalyzePodcast compares every transcript in a podcast directory and records
// the text that recurs in at least MinEpisodes episodes in the directory's
// podcast.json. Entries a person disabled or added by hand survive a rerun.
func AnalyzePodcast(dir string, opts AnalyzeOptions) (AnalyzeResult, error) {
	res := AnalyzeResult{Dir: dir}
	if opts.MinEpisodes <= 0 {
		opts.MinEpisodes = DefaultBoilerplateEpisodes
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.transcript.json"))
	if err != nil {
		return res, err
	}
	res.Transcripts = len(paths)
	if len(paths) < MinAnalyzeTranscripts {
		res.Skipped = fmt.Sprintf("only %d transcripts; at least %d are needed", len(paths), MinAnalyzeTranscripts)
		return res, nil
	}
	repeatOpts := detect.RepeatOptions{Shingle: 8, Gap: 6, MinWords: opts.MinWords}
	corpus, err := corpusOf(paths, repeatOpts)
	if err != nil {
		return res, err
	}
	found := corpus.BoilerplatePhrases(opts.MinEpisodes)

	cfg, err := config.LoadPodcastConfigErr(dir, config.PodcastConfig{})
	if err != nil {
		return res, err
	}
	res.Phrases, res.Added, res.Dropped = mergePhrases(cfg.Boilerplate, found, repeatOpts)
	if opts.DryRun {
		return res, nil
	}
	cfg.Boilerplate = res.Phrases
	return res, config.SavePodcastConfig(dir, cfg)
}

func corpusOf(paths []string, opts detect.RepeatOptions) (*detect.Corpus, error) {
	sort.Strings(paths)
	corpus := detect.NewCorpus(opts)
	for _, path := range paths {
		td, err := LoadTranscriptFile(path)
		if err != nil {
			return nil, err
		}
		corpus.Add(strings.TrimSuffix(filepath.Base(path), ".transcript.json"), detect.WordsOf(td))
	}
	return corpus, nil
}

// mergePhrases combines the phrases already on file with a fresh analysis.
// A reworded version of a disabled phrase stays disabled, and manual entries are
// never dropped; other entries the analysis no longer supports are removed.
func mergePhrases(existing []config.BoilerplatePhrase, found []detect.Phrase, opts detect.RepeatOptions) (merged []config.BoilerplatePhrase, added, dropped int) {
	matched := make([]bool, len(existing))
	for _, phrase := range found {
		entry := config.BoilerplatePhrase{Text: phrase.Text, Episodes: phrase.Episodes, Position: positionName(phrase.Position)}
		if i := similarEntry(existing, phrase.Text, opts); i >= 0 {
			matched[i] = true
			entry.Disabled, entry.Manual = existing[i].Disabled, existing[i].Manual
		} else {
			added++
		}
		merged = append(merged, entry)
	}
	for i, old := range existing {
		if matched[i] {
			continue
		}
		if old.Manual || old.Disabled {
			merged = append(merged, old)
		} else {
			dropped++
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Episodes > merged[j].Episodes })
	return merged, added, dropped
}

func similarEntry(existing []config.BoilerplatePhrase, text string, opts detect.RepeatOptions) int {
	for i, e := range existing {
		if detect.SimilarPhrases(e.Text, text, opts) {
			return i
		}
	}
	return -1
}

func positionName(frac float64) string {
	switch {
	case frac < 0.1:
		return "start"
	case frac > 0.85:
		return "end"
	}
	return "mid"
}

// BoilerplateCuts returns the stretches of a transcript that match the
// podcast's recorded boilerplate. It reads podcast.json in podDir, so a show
// never analysed simply has none.
func BoilerplateCuts(podDir string, td *types.TranscriptionData) []types.AdSegment {
	cfg, err := config.LoadPodcastConfigErr(podDir, config.PodcastConfig{})
	if err != nil || td == nil {
		return nil
	}
	var texts []string
	for _, p := range cfg.Boilerplate {
		if !p.Disabled && strings.TrimSpace(p.Text) != "" {
			texts = append(texts, p.Text)
		}
	}
	if len(texts) == 0 {
		return nil
	}
	matcher := detect.NewPhraseMatcher(texts, detect.RepeatOptions{Shingle: 8, Gap: 6, MinWords: 12})
	var cuts []types.AdSegment
	for _, r := range matcher.Find(detect.WordsOf(td)) {
		cuts = append(cuts, types.AdSegment{Start: r.Start, End: r.End, Reason: "boilerplate"})
	}
	return cuts
}

// WithoutSpans returns a copy of a transcript minus the segments whose middle
// falls inside any span, so a detector is not asked about text already cut.
func WithoutSpans(td *types.TranscriptionData, spans []types.AdSegment) *types.TranscriptionData {
	if td == nil || len(spans) == 0 {
		return td
	}
	out := *td
	out.Segments = nil
	for _, seg := range td.Segments {
		mid := (seg.Start + seg.End) / 2
		if !insideAny(mid, spans) {
			out.Segments = append(out.Segments, seg)
		}
	}
	return &out
}

func insideAny(t float64, spans []types.AdSegment) bool {
	for _, s := range spans {
		if t >= s.Start && t <= s.End {
			return true
		}
	}
	return false
}
