package podcast

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"pod/pkg/backend"
)

var updateMatchGolden = flag.Bool("update-match", false, "rewrite testdata/match.txt")

// TestMatchGolden pins how every podcast query resolves. The matchers decide
// which podcast a command acts on, so a silent change here silently retargets
// user commands; this records the whole table instead of spot-checking it.
// Regenerate with: go test ./pkg/podcast -update-match
func TestMatchGolden(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mk := func(folder, title string, fav bool) {
		d := filepath.Join(root, folder)
		_ = os.MkdirAll(d, 0755)
		_ = os.WriteFile(filepath.Join(d, "ep.mp3"), []byte("x"), 0644)
		cfg := fmt.Sprintf(`{"title":%q,"favorite":%t}`, title, fav)
		_ = os.WriteFile(filepath.Join(d, "podcast.json"), []byte(cfg), 0644)
	}
	mk("Daily_Show", "The Daily Show", true)
	mk("Daily_News", "Daily News Hour", false)
	mk("Lex_Fridman", "Lex Fridman Podcast", true)
	mk("Solo", "Solo Show", false)
	mk("Numbers123", "123 Numbers", false)

	entries := ScanPodcastDirs(root)
	sort.Slice(entries, func(i, j int) bool { return entries[i].FolderName < entries[j].FolderName })

	pods := []backend.Podcast{}
	for _, e := range entries {
		p := backend.Podcast{ID: "id-" + e.FolderName, RelPath: e.FolderName, Path: e.Dir}
		p.Media.Metadata.Title = e.Title
		p.Media.ID = "media-" + e.FolderName
		pods = append(pods, p)
	}

	queries := []string{
		"", "  ", "daily", "Daily", "DAILY", "fridman", "solo", "nope",
		"Daily_Show", "daily_show", "1", "3", "99", "0", "-1",
		"lex.*man", "^Solo$", "[", "123", "Numbers123",
		"all", "*", "fav", "favorites", "not-fav", "non-favorites", "unfav",
	}

	var b strings.Builder
	for _, q := range queries {
		fmt.Fprintf(&b, "=== query %q\n", q)

		m, err := MatchLocalPodcasts(entries, q)
		fmt.Fprintf(&b, "  MatchLocalPodcasts  -> %s | err=%s\n", localStr(m), errStr(err))

		bm, err := MatchBackendPodcasts(pods, q)
		fmt.Fprintf(&b, "  MatchBackendPodcast -> %s | err=%s\n", backendStr(bm), errStr(err))

		g, err := ResolvePodcastGroup(root, q)
		fmt.Fprintf(&b, "  ResolvePodcastGroup -> %s | err=%s\n", groupStr(g), errStr(err))

		bg, err := resolveBackendPodcastGroup(pods, root, q)
		fmt.Fprintf(&b, "  ResolveBackendGroup -> %s | err=%s\n", bgroupStr(bg), errStr(err))

		kind, ok := parsePodcastGroupKind(q)
		fmt.Fprintf(&b, "  ParseGroupKind      -> %q ok=%t\n", kind, ok)

		for _, name := range []string{"The Daily Show", "Lex Fridman Podcast", "123 Numbers", ""} {
			fmt.Fprintf(&b, "  matchesPodcastName(%q) -> %t\n", name, matchesPodcastName(name, q))
		}
	}
	path := filepath.Join("testdata", "match.txt")
	got := b.String()
	if *updateMatchGolden {
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run: go test ./pkg/podcast -update-match)", err)
	}
	if got != string(want) {
		t.Errorf("podcast query resolution changed; diff testdata/match.txt against:\n%s", got)
	}
}

func errStr(err error) string {
	if err == nil {
		return "<nil>"
	}
	return strings.ReplaceAll(err.Error(), "\n", " / ")
}

func localStr(p *PodcastDirEntry) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s/%s", p.FolderName, p.Title)
}

func backendStr(p *backend.Podcast) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s/%s", p.ID, backendPodcastTitle(*p))
}

func groupStr(g *ResolvedPodcastGroup) string {
	if g == nil {
		return "<nil>"
	}
	var names []string
	for _, e := range g.Entries {
		names = append(names, e.FolderName)
	}
	sort.Strings(names)
	return fmt.Sprintf("kind=%s label=%q entries=[%s]", g.Kind, g.Label, strings.Join(names, ","))
}

func bgroupStr(g *ResolvedBackendGroup) string {
	if g == nil {
		return "<nil>"
	}
	var names []string
	for _, p := range g.Podcasts {
		names = append(names, p.ID)
	}
	sort.Strings(names)
	return fmt.Sprintf("kind=%s label=%q podcasts=[%s]", g.Kind, g.Label, strings.Join(names, ","))
}
