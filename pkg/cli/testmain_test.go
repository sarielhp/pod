package cli

import (
	"os"
	"testing"
	"time"

	"pod/pkg/podcast"
	"pod/pkg/podcast/podtest"
)

func TestMain(m *testing.M) {
	os.Exit(podtest.IsolateMain(m, func() {
		zero := time.Duration(0)
		podcast.SetFeedRetryDelay(&zero)
		podcast.SetFeedTransport(podtest.OfflineTransport())
		podcast.SetAllowPrivateHosts(true)
	}))
}
