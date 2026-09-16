package cli

import (
	"strings"
	"testing"
)

func TestRSSGenIsATopLevelCommand(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	app := buildCLIApp(&action, &opts)

	var found bool
	for _, c := range app.Commands {
		if c.Name == "rss_gen" {
			found = true
		}
		// Publishing the site should not also hide under `server`, or there
		// are two names for one act.
		if c.Name == "server" {
			for _, sub := range c.Subcommands {
				if sub.Name == "rss_gen" || sub.Name == "feed" {
					t.Errorf("server still carries a %q subcommand", sub.Name)
				}
			}
		}
	}
	if !found {
		t.Error("rss_gen is not a top-level command")
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
