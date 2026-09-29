package detect

import "sort"

// Phrase is text that recurs across a show's episodes.
type Phrase struct {
	Text string
	// Episodes is how many episodes of the show carry it, counting the one the
	// phrase was read from.
	Episodes int
	// Position is where in its episode the phrase was read, from 0 (start) to 1.
	Position float64
	Words    int
	// Seconds is how long the passage runs in the episode it was read from.
	Seconds float64
}

// BoilerplatePhrases returns one representative phrase for each distinct text
// that recurs in at least minEpisodes episodes. Variants of the same text, such
// as credits that differ only by a guest's name, collapse into the most widely
// repeated one, so the list stays short enough to read.
func (c *Corpus) BoilerplatePhrases(minEpisodes int) []Phrase {
	var candidates []candidatePhrase
	for id := range c.episodes {
		for _, r := range c.Repeats(id, nil) {
			if r.Episodes+1 >= minEpisodes {
				candidates = append(candidates, c.candidateFor(id, r))
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Episodes != b.Episodes {
			return a.Episodes > b.Episodes
		}
		if a.Words != b.Words {
			return a.Words > b.Words
		}
		return a.Text < b.Text
	})
	return dedupePhrases(candidates)
}

type candidatePhrase struct {
	Phrase
	hashes []uint64
}

func (c *Corpus) candidateFor(id int, r Repeat) candidatePhrase {
	ep := c.episodes[id]
	span := ep.words[r.StartWord : r.EndWord+1]
	dur := ep.words[len(ep.words)-1].End
	return candidatePhrase{
		Phrase: Phrase{Text: r.Text, Episodes: r.Episodes + 1, Position: r.Start / max(dur, 1), Words: len(span), Seconds: r.End - r.Start},
		hashes: shingleHashes(span, c.opts.Shingle),
	}
}

// dedupePhrases keeps a phrase only when at least half of it is new, given the
// phrases already kept. The input order decides which variant represents a group.
func dedupePhrases(candidates []candidatePhrase) []Phrase {
	covered := map[uint64]bool{}
	var out []Phrase
	for _, cand := range candidates {
		seen := 0
		for _, h := range cand.hashes {
			if covered[h] {
				seen++
			}
		}
		if len(cand.hashes) == 0 || seen*2 >= len(cand.hashes) {
			continue
		}
		for _, h := range cand.hashes {
			covered[h] = true
		}
		out = append(out, cand.Phrase)
	}
	return out
}

// PhraseOverlap is the fraction of the shorter text's word runs that also occur
// in the other, from 0 (nothing shared) to 1 (one contains the other). It is how
// a reworded version of a phrase is recognised as the same phrase.
func PhraseOverlap(a, b string, opts RepeatOptions) float64 {
	k := NewCorpus(opts).opts.Shingle
	ha, hb := shingleHashes(wordsOfText(a), k), shingleHashes(wordsOfText(b), k)
	if len(ha) == 0 || len(hb) == 0 {
		return 0
	}
	set := make(map[uint64]bool, len(hb))
	for _, h := range hb {
		set[h] = true
	}
	shared := 0
	for _, h := range ha {
		if set[h] {
			shared++
		}
	}
	return float64(shared) / float64(min(len(ha), len(hb)))
}

// PhraseMatcher finds known phrases in a transcript. It is the cheap half of
// boilerplate removal: the recurring texts are learned once from a show's back
// catalogue, and each new episode needs only this lookup.
type PhraseMatcher struct {
	opts RepeatOptions
	set  map[uint64]bool
}

// NewPhraseMatcher indexes the phrases. Zero option fields take their defaults.
func NewPhraseMatcher(phrases []string, opts RepeatOptions) *PhraseMatcher {
	opts = NewCorpus(opts).opts
	m := &PhraseMatcher{opts: opts, set: map[uint64]bool{}}
	for _, p := range phrases {
		for _, h := range shingleHashes(wordsOfText(p), opts.Shingle) {
			m.set[h] = true
		}
	}
	return m
}

// Find returns the stretches of words that belong to a known phrase.
func (m *PhraseMatcher) Find(words []Word) []Repeat {
	hashes := shingleHashes(words, m.opts.Shingle)
	hit := make([]bool, len(words))
	for i, h := range hashes {
		if !m.set[h] {
			continue
		}
		for j := i; j < i+m.opts.Shingle; j++ {
			hit[j] = true
		}
	}
	var out []Repeat
	for _, r := range mergeRuns(hit, m.opts.Gap) {
		if r[1]-r[0]+1 < m.opts.MinWords {
			continue
		}
		out = append(out, Repeat{StartWord: r[0], EndWord: r[1], Start: words[r[0]].Start, End: words[r[1]].End})
	}
	return out
}

func wordsOfText(text string) []Word {
	tokens := tokenize(text)
	words := make([]Word, len(tokens))
	for i, t := range tokens {
		words[i] = Word{Text: t}
	}
	return words
}
