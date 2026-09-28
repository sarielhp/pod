package gemini

import (
	"os"
	"testing"

	"pod/pkg/podcast/podtest"
)

func TestMain(m *testing.M) {
	os.Exit(podtest.IsolateMain(m, nil))
}
