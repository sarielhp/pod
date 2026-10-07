package podcastsearch

import (
	"net/http"
	"os"
	"testing"

	"pod/pkg/podcast/podtest"
)

func TestMain(m *testing.M) {
	os.Exit(podtest.IsolateMain(m, func() {
		http.DefaultTransport = podtest.OfflineTransport()
	}))
}
