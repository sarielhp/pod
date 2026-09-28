package detect

import (
	"os"
	"testing"
	"time"

	"pod/pkg/podcast/podtest"
)

func TestMain(m *testing.M) {
	os.Exit(podtest.IsolateMain(m, func() {
		SetRetryBackoff(func(int) time.Duration { return 0 })
	}))
}
