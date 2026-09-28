package podcast

import (
	"os"
	"testing"
	"time"

	"pod/pkg/podcast/podtest"
)

func TestMain(m *testing.M) {
	os.Exit(podtest.IsolateMain(m, func() {
		zero := time.Duration(0)
		SetFeedRetryDelay(&zero)
		SetFeedTransport(podtest.OfflineTransport())
		SetAllowPrivateHosts(true)
	}))
}
