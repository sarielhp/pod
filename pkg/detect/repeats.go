package detect

import (
	"hash/fnv"
	"math/rand"
	"slices"
	"strings"
	"unicode"

	"pod/pkg/types"
)

// Word is one spoken word of a transcript with an estimated time. Transcripts
// time whole segments, so a word's time is interpolated within its segment.
type Word struct {
	Text       string
	Start, End float64
}

// RepeatOptions tunes what counts as a repeated span.
type RepeatOptions struct {
	// Shingle is how many consecutive words must match to start a match.
	Shingle int
	// Gap is how many unmatched words a match may span. Speech recognition
	// changes a word here and there, and a repeated read should stay one span.
	Gap int
	// MinWords drops spans shorter than this, which are usually a stock phrase
	// rather than a repeated read.
	MinWords int
}

// DefaultRepeatOptions are the values that worked on real podcast libraries.
// Span length between 12 and 40 words changed results very little.
func DefaultRepeatOptions() RepeatOptions {
	return RepeatOptions{Shingle: 8, Gap: 6, MinWords: 20}
}

// Repeat is a stretch of an episode whose words also appear, in the same
// order, in other episodes of the same show.
type Repeat struct {
	StartWord, EndWord int
	Start, End         float64
	// Episodes is how many other episodes carry the span, taken as the median
	// over its shingles so one common phrase inside it does not inflate it.
	Episodes int
	Text     string
}

// WordsOf turns a transcript into normalised words: lower case, letters,
// digits and apostrophes only, so punctuation and capitalisation differences
// between two recognitions of the same audio do not break a match.
func WordsOf(td *types.TranscriptionData) []Word {
	var words []Word
	if td == nil {
		return words
	}
	for _, seg := range td.Segments {
		tokens := strings.FieldsFunc(strings.ToLower(seg.Text), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\''
		})
		if len(tokens) == 0 {
			continue
		}
		step := (seg.End - seg.Start) / float64(len(tokens))
		for i, tok := range tokens {
			start := seg.Start + float64(i)*step
			words = append(words, Word{Text: tok, Start: start, End: start + step})
		}
	}
	return words
}

type corpusEpisode struct {
	name   string
	words  []Word
	hashes []uint64
}

// Corpus indexes the episodes of one show so each can be compared with all the
// others at once.
type Corpus struct {
	opts     RepeatOptions
	episodes []corpusEpisode
	index    map[uint64][]int32
}

// NewCorpus returns an empty corpus. Zero option fields take their defaults.
func NewCorpus(opts RepeatOptions) *Corpus {
	def := DefaultRepeatOptions()
	if opts.Shingle <= 0 {
		opts.Shingle = def.Shingle
	}
	if opts.Gap < 0 {
		opts.Gap = def.Gap
	}
	if opts.MinWords <= 0 {
		opts.MinWords = def.MinWords
	}
	return &Corpus{opts: opts, index: make(map[uint64][]int32)}
}

// Add indexes an episode and returns its position in the corpus.
func (c *Corpus) Add(name string, words []Word) int {
	ep := corpusEpisode{name: name, words: words, hashes: shingleHashes(words, c.opts.Shingle)}
	id := int32(len(c.episodes))
	c.episodes = append(c.episodes, ep)
	seen := make(map[uint64]bool, len(ep.hashes))
	for _, h := range ep.hashes {
		if seen[h] {
			continue
		}
		seen[h] = true
		c.index[h] = append(c.index[h], id)
	}
	return int(id)
}

// Len is the number of episodes in the corpus.
func (c *Corpus) Len() int { return len(c.episodes) }

// Name is the name an episode was added under.
func (c *Corpus) Name(id int) string { return c.episodes[id].name }

// Words are the normalised words of an episode.
func (c *Corpus) Words(id int) []Word { return c.episodes[id].words }

// Repeats finds the spans of episode target that also occur in the partner
// episodes. A nil partners slice means every other episode.
func (c *Corpus) Repeats(target int, partners []int) []Repeat {
	ep := c.episodes[target]
	counts := c.shingleCounts(target, partners)
	hit := make([]bool, len(ep.words))
	for i, n := range counts {
		if n == 0 {
			continue
		}
		for j := i; j < i+c.opts.Shingle; j++ {
			hit[j] = true
		}
	}
	var out []Repeat
	for _, r := range mergeRuns(hit, c.opts.Gap) {
		if r[1]-r[0]+1 >= c.opts.MinWords {
			out = append(out, c.repeatFor(ep, counts, r))
		}
	}
	return out
}

func (c *Corpus) shingleCounts(target int, partners []int) []int {
	ep := c.episodes[target]
	var allowed []bool
	if partners != nil {
		allowed = make([]bool, len(c.episodes))
		for _, p := range partners {
			allowed[p] = true
		}
	}
	counts := make([]int, len(ep.hashes))
	for i, h := range ep.hashes {
		for _, other := range c.index[h] {
			if int(other) != target && (allowed == nil || allowed[other]) {
				counts[i]++
			}
		}
	}
	return counts
}

func (c *Corpus) repeatFor(ep corpusEpisode, counts []int, run [2]int) Repeat {
	var per []int
	for i := run[0]; i <= run[1]-c.opts.Shingle+1 && i < len(counts); i++ {
		if counts[i] > 0 {
			per = append(per, counts[i])
		}
	}
	slices.Sort(per)
	texts := make([]string, 0, run[1]-run[0]+1)
	for _, w := range ep.words[run[0] : run[1]+1] {
		texts = append(texts, w.Text)
	}
	r := Repeat{
		StartWord: run[0], EndWord: run[1],
		Start: ep.words[run[0]].Start, End: ep.words[run[1]].End,
		Text: strings.Join(texts, " "),
	}
	if len(per) > 0 {
		r.Episodes = per[len(per)/2]
	}
	return r
}

// SamplePartners picks n partner episodes for target, reproducibly, so a
// measurement of "how much do n other episodes find" can be repeated.
func (c *Corpus) SamplePartners(target, n int, seed int64) []int {
	var others []int
	for i := range c.episodes {
		if i != target {
			others = append(others, i)
		}
	}
	rng := rand.New(rand.NewSource(seed + int64(target)))
	rng.Shuffle(len(others), func(i, j int) { others[i], others[j] = others[j], others[i] })
	if n < len(others) {
		others = others[:n]
	}
	return others
}

// RepeatSegments converts spans to ad segments so they can be scored with
// ScoreSegments like any other detection.
func RepeatSegments(repeats []Repeat) []types.AdSegment {
	segs := make([]types.AdSegment, 0, len(repeats))
	for _, r := range repeats {
		segs = append(segs, types.AdSegment{Start: r.Start, End: r.End, Reason: "repeated text"})
	}
	return segs
}

func shingleHashes(words []Word, k int) []uint64 {
	if len(words) < k {
		return nil
	}
	out := make([]uint64, 0, len(words)-k+1)
	h := fnv.New64a()
	for i := 0; i+k <= len(words); i++ {
		h.Reset()
		for _, w := range words[i : i+k] {
			h.Write([]byte(w.Text))
			h.Write([]byte{0})
		}
		out = append(out, h.Sum64())
	}
	return out
}

func mergeRuns(hit []bool, gap int) [][2]int {
	var runs [][2]int
	for i := 0; i < len(hit); {
		if !hit[i] {
			i++
			continue
		}
		j := i
		for j < len(hit) && hit[j] {
			j++
		}
		if n := len(runs); n > 0 && i-runs[n-1][1]-1 <= gap {
			runs[n-1][1] = j - 1
		} else {
			runs = append(runs, [2]int{i, j - 1})
		}
		i = j
	}
	return runs
}
