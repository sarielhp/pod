package podcast

import "testing"

func TestSubscriptionMatches(t *testing.T) {
	t.Parallel()
	sub := Subscription{ID: "ab12", Title: "The Daily Show"}

	cases := []struct {
		query string
		want  bool
		why   string
	}{
		{"", true, "empty query selects everything"},
		{"ab12", true, "exact id"},
		{"AB12", true, "id is case-insensitive"},
		{"daily", true, "title substring"},
		{"DAILY SHOW", true, "title substring is case-insensitive"},
		{"The Daily Show", true, "full title"},
		{"ab1", false, "id must match exactly, not by prefix"},
		{"weekly", false, "unrelated"},
		{"dai.*show", false, "regex is not supported here, unlike matchesPodcastName"},
	}
	for _, c := range cases {
		if got := SubscriptionMatches(sub, c.query); got != c.want {
			t.Errorf("SubscriptionMatches(%q) = %t, want %t (%s)", c.query, got, c.want, c.why)
		}
	}
}

// The empty-title case used to be reachable through the inline copies in the
// CLI; keep it defined rather than accidental.
func TestSubscriptionMatchesEmptyTitleStillMatchesByID(t *testing.T) {
	t.Parallel()
	sub := Subscription{ID: "zz99"}
	if !SubscriptionMatches(sub, "zz99") {
		t.Error("id match should not depend on a title being set")
	}
	if !SubscriptionMatches(sub, "") {
		t.Error("empty query should still select")
	}
}
