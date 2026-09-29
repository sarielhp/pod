package detect

import (
	"strings"
	"testing"
)

func TestShingleHashesGroupWindowsByContentAndOrder(t *testing.T) {
	text := "a b c d e f a b c d e f b a c d e f"
	var words []Word
	for _, w := range strings.Fields(text) {
		words = append(words, Word{Text: w})
	}
	for _, k := range []int{1, 3, 6, 64} {
		got := shingleHashes(words, k)
		if len(words) < k {
			if got != nil {
				t.Fatalf("k=%d: want no hashes", k)
			}
			continue
		}
		if len(got) != len(words)-k+1 {
			t.Fatalf("k=%d: got %d hashes", k, len(got))
		}
		for i := range got {
			for j := range got {
				same := strings.Join(fields(words[i:i+k]), " ") == strings.Join(fields(words[j:j+k]), " ")
				if same != (got[i] == got[j]) {
					t.Fatalf("k=%d windows %d and %d: same text %v but hashes equal %v", k, i, j, same, got[i] == got[j])
				}
			}
		}
	}
}

func fields(ws []Word) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.Text
	}
	return out
}
