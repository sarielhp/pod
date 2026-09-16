package cli

import (
	"strings"
	"testing"
)

func TestRetiredFeedNameRefusesRatherThanRunningFeeds(t *testing.T) {
	t.Parallel()
	// Removing `feed` outright would leave it prefix-matching `feeds`, so the
	// old name would quietly run the upstream check — the opposite action.
	cmd := buildServerFeedRetiredSubcommand()
	if cmd.Name != "feed" {
		t.Fatalf("stub name = %q", cmd.Name)
	}
	if !cmd.Hidden {
		t.Error("a retired name should not be advertised in help")
	}
	err := cmd.Run(nil)
	if err == nil {
		t.Fatal("the retired name succeeded")
	}
	if !strings.Contains(err.Error(), "rss_gen") {
		t.Errorf("the error does not name the replacement: %v", err)
	}
}

func TestRSSGenIsTheNameThatWorks(t *testing.T) {
	t.Parallel()
	var action string
	var opts CLIOptions
	cmd := buildServerRSSGenSubcommand(&opts, &action)
	if cmd.Name != "rss_gen" {
		t.Errorf("name = %q, want rss_gen", cmd.Name)
	}
	if cmd.Hidden {
		t.Error("the working command should appear in help")
	}
}
