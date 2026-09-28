package podtest

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(IsolateMain(m, nil))
}
