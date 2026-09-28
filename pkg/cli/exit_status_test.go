package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"pod/pkg/podcast"
)

func TestDownloadFailuresBecomeAnError(t *testing.T) {
	if err := downloadFailuresError(podcast.SubscriptionDownloadResult{Downloaded: 3}); err != nil {
		t.Fatalf("no failures, got %v", err)
	}
	res := podcast.SubscriptionDownloadResult{Downloaded: 1, Failures: []error{errors.New("Show A: HTTP 404"), errors.New("Show B: timeout")}}
	err := downloadFailuresError(res)
	if err == nil || !strings.Contains(err.Error(), "2 podcast download(s) failed") {
		t.Fatalf("two failed downloads reported as success: %v", err)
	}
}

func TestFlushAsksBeforeDeletingUnlessForced(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		force bool
		want  bool
	}{
		{"default is no", "\n", false, false},
		{"explicit no", "n\n", false, false},
		{"yes", "y\n", false, true},
		{"YES", "YES\n", false, true},
		{"eof", "", false, false},
	}
	for _, tc := range cases {
		var out bytes.Buffer
		cli := CLIOptions{In: strings.NewReader(tc.in), Out: &out}
		cli.ForceDelete = tc.force
		got, err := confirmFlush(cli, "Show", 4)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: confirmed=%v, want %v", tc.name, got, tc.want)
		}
		if !strings.Contains(out.String(), "4 audio file(s)") {
			t.Fatalf("%s: prompt does not name the count: %q", tc.name, out.String())
		}
	}
}

func TestFlushCommandAcceptsForce(t *testing.T) {
	var action string
	var opts CLIOptions
	if err := buildCLIApp(&action, &opts).Execute([]string{"server", "flush", "123", "--force"}); err != nil {
		t.Fatal(err)
	}
	if !opts.ForceDelete {
		t.Fatal("--force was not parsed")
	}
}
