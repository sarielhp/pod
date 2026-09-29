package detect

import (
	"fmt"
	"strings"
	"testing"

	"pod/pkg/types"
)

func transcriptOf(texts ...string) *types.TranscriptionData {
	td := &types.TranscriptionData{}
	for i, text := range texts {
		td.Segments = append(td.Segments, types.TranscriptionSegment{Start: float64(i * 10), End: float64(i*10 + 10), Text: text})
	}
	return td
}

func filler(prefix string, n int) string {
	words := make([]string, n)
	for i := range words {
		words[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return strings.Join(words, " ")
}

const sponsorRead = "this episode is brought to you by acme widgets the finest widgets money can buy visit acme dot com slash pod and use code pod for ten percent off your first order today"

func TestWordsOfNormalisesAndTimes(t *testing.T) {
	t.Parallel()
	words := WordsOf(transcriptOf("Hello, WORLD! It's fine.", "  "))
	got := make([]string, len(words))
	for i, w := range words {
		got[i] = w.Text
	}
	if strings.Join(got, " ") != "hello world it's fine" {
		t.Fatalf("unexpected words %q", got)
	}
	if words[0].Start != 0 || words[3].End != 10 {
		t.Errorf("times should spread across the segment, got %+v", words)
	}
}

func TestRepeatsFindsSharedReadAndTimesIt(t *testing.T) {
	t.Parallel()
	c := NewCorpus(RepeatOptions{})
	a := c.Add("a", WordsOf(transcriptOf(filler("intro", 40), sponsorRead, filler("body", 60))))
	c.Add("b", WordsOf(transcriptOf(filler("other", 50), sponsorRead, filler("tail", 30))))
	c.Add("c", WordsOf(transcriptOf(sponsorRead, filler("more", 40))))

	got := c.Repeats(a, nil)
	if len(got) != 1 {
		t.Fatalf("want one repeated span, got %+v", got)
	}
	if got[0].Episodes != 2 {
		t.Errorf("the read is in two other episodes, got %d", got[0].Episodes)
	}
	if got[0].Start < 10 || got[0].Start > 20 || !strings.HasPrefix(got[0].Text, "this episode is brought") {
		t.Errorf("span should start at the read (second segment), got %+v", got[0])
	}
}

func TestRepeatsHonoursPartnersAndMinWords(t *testing.T) {
	t.Parallel()
	c := NewCorpus(RepeatOptions{})
	a := c.Add("a", WordsOf(transcriptOf(sponsorRead, filler("x", 30))))
	b := c.Add("b", WordsOf(transcriptOf(sponsorRead)))
	other := c.Add("c", WordsOf(transcriptOf(filler("unrelated", 40))))

	if len(c.Repeats(a, []int{other})) != 0 {
		t.Error("a partner without the read must not produce a match")
	}
	if len(c.Repeats(a, []int{b})) != 1 {
		t.Error("the partner with the read should match")
	}
	short := NewCorpus(RepeatOptions{MinWords: 200})
	x := short.Add("x", WordsOf(transcriptOf(sponsorRead)))
	short.Add("y", WordsOf(transcriptOf(sponsorRead)))
	if len(short.Repeats(x, nil)) != 0 {
		t.Error("a span below MinWords must be dropped")
	}
}

func TestRepeatsBridgesSmallRecognitionDifferences(t *testing.T) {
	t.Parallel()
	changed := strings.Replace(sponsorRead, "finest widgets", "very finest widgets", 1)
	c := NewCorpus(RepeatOptions{})
	a := c.Add("a", WordsOf(transcriptOf(sponsorRead)))
	c.Add("b", WordsOf(transcriptOf(changed)))
	if got := c.Repeats(a, nil); len(got) != 1 {
		t.Fatalf("one inserted word should not split the span, got %d spans", len(got))
	}
}

func TestSamplePartnersIsReproducibleAndExcludesTarget(t *testing.T) {
	t.Parallel()
	c := NewCorpus(RepeatOptions{})
	for i := 0; i < 10; i++ {
		c.Add(fmt.Sprint(i), WordsOf(transcriptOf(filler("e", 5))))
	}
	one, two := c.SamplePartners(3, 4, 7), c.SamplePartners(3, 4, 7)
	if fmt.Sprint(one) != fmt.Sprint(two) || len(one) != 4 {
		t.Fatalf("sampling must be reproducible: %v vs %v", one, two)
	}
	for _, p := range c.SamplePartners(3, 100, 7) {
		if p == 3 {
			t.Fatal("the target is never its own partner")
		}
	}
}
