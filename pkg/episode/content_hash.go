package episode

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// ContentHash returns a short, stable hex digest of a file's bytes.
//
// It is recorded when a cleaned episode is written and republished in the
// feed's enclosure URL as a version token, so the URL names the exact audio
// it serves and any HTTP cache keyed on it cannot hand out stale bytes.
// Deriving the token from the bytes means it changes exactly when the audio
// does. It does not make a client re-download an episode it already holds.
func ContentHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}
