package cli

import (
	"strings"
	"testing"
)

func TestGenRSSIsATopLevelCommand(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	var found bool
	for _, c := range app.Commands {
		if c.Name == "gen_rss" {
			found = true
		}
		// Publishing the site should not also hide under `server`, or there
		// are two names for one act.
		if c.Name == "server" {
			for _, sub := range c.Subcommands {
				if sub.Name == "gen_rss" || sub.Name == "feed" {
					t.Errorf("server still carries a %q subcommand", sub.Name)
				}
			}
		}
	}
	if !found {
		t.Error("gen_rss is not a top-level command")
	}
}

func TestServerFeedPrefixReachesFeeds(t *testing.T) {
	t.Parallel()
	// With no `feed` subcommand of its own, `pod server feed` is an
	// unambiguous prefix of `feeds`, which is the intended behaviour: the two
	// are no longer separate commands one letter apart.
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)
	for _, c := range app.Commands {
		if c.Name != "server" {
			continue
		}
		var matches []string
		for _, sub := range c.Subcommands {
			if strings.HasPrefix(sub.Name, "feed") {
				matches = append(matches, sub.Name)
			}
		}
		if len(matches) != 1 || matches[0] != "feeds" {
			t.Errorf("`server feed` is ambiguous or missing: %v", matches)
		}
	}
}

func TestEveryTopLevelCommandHasAUniqueInitial(t *testing.T) {
	t.Parallel()
	// Renaming tui to ui and rss_gen to gen_rss freed the letters t and r,
	// which had each been shared by two commands. Single-letter abbreviation
	// works again, and this is what keeps it working.
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	byInitial := map[byte][]string{}
	for _, c := range app.Commands {
		if c.Name == "" || c.Hidden {
			continue
		}
		byInitial[c.Name[0]] = append(byInitial[c.Name[0]], c.Name)
	}
	for initial, names := range byInitial {
		if len(names) > 1 {
			t.Errorf("%q abbreviates %d commands: %v", string(initial), len(names), names)
		}
	}
}

func TestGenRSSRequestShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args    []string
		podcast string
		count   int
		ok      bool
		what    string
	}{
		{nil, "", 0, true, "no arguments republishes everything"},
		{[]string{"10"}, "", 10, true, "a bare number is a count across the library"},
		{[]string{"daily blast"}, "daily blast", 0, true, "a name republishes one show"},
		{[]string{"daily blast", "10"}, "daily blast", 10, true, "a name and a number scope the count to that show"},
		{[]string{"tdbwg", "3"}, "tdbwg", 3, true, "the short id works as the name"},
		{[]string{"daily blast", "0"}, "", 0, false, "a non-positive count is refused"},
		{[]string{"daily blast", "soon"}, "", 0, false, "a non-numeric second argument is refused"},
		{[]string{"a", "1", "b"}, "", 0, false, "three arguments are refused"},
	}
	for _, c := range cases {
		pod, n, ok := genRSSRequest(c.args)
		if ok != c.ok || pod != c.podcast || n != c.count {
			t.Errorf("%s: genRSSRequest(%q) = (%q, %d, %v), want (%q, %d, %v)",
				c.what, c.args, pod, n, ok, c.podcast, c.count, c.ok)
		}
	}
}
