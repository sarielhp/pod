package adremoval

import (
	"os"
	"testing"
	"time"

	"pod/pkg/detect"
	"pod/pkg/podcast/podtest"
)

func TestMain(m *testing.M) {
	os.Exit(podtest.IsolateMain(m, func() {
		detect.SetRetryBackoff(func(int) time.Duration { return 0 })
	}))
}
