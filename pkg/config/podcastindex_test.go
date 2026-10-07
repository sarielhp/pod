package config

import (
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/types"
)

// clearPodcastIndexEnv keeps a developer's own credentials out of the tests:
// environment variables win over the file, so an exported key would otherwise
// mask what the file says.
func clearPodcastIndexEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PODCASTINDEX_API_KEY", "")
	t.Setenv("PODCASTINDEX_API_SECRET", "")
	t.Setenv("PODCASTINDEX_API_KEY_FILE", "")
}

func TestResolvePodcastIndexCredentialsFromJSON(t *testing.T) {
	clearPodcastIndexEnv(t)
	path := filepath.Join(t.TempDir(), "podcast_index_key.json")
	if err := os.WriteFile(path, []byte(`{"api_key":"the-key","api_secret":"the-secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &types.Config{PodcastIndexConfig: types.PodcastIndexConfig{PodcastIndexAPIKeyFile: path}}

	key, secret := ResolvePodcastIndexCredentials(cfg)
	if key != "the-key" || secret != "the-secret" {
		t.Fatalf("got key=%q secret=%q", key, secret)
	}
}

func TestResolvePodcastIndexCredentialsFromLabelledText(t *testing.T) {
	clearPodcastIndexEnv(t)
	path := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(path, []byte("Key: alpha\nSecret: beta\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &types.Config{PodcastIndexConfig: types.PodcastIndexConfig{PodcastIndexAPIKeyFile: path}}

	key, secret := ResolvePodcastIndexCredentials(cfg)
	if key != "alpha" || secret != "beta" {
		t.Fatalf("got key=%q secret=%q", key, secret)
	}
}

func TestResolvePodcastIndexCredentialsEnvWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(path, []byte(`{"api_key":"file-key","api_secret":"file-secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PODCASTINDEX_API_KEY", "env-key")
	t.Setenv("PODCASTINDEX_API_SECRET", "env-secret")
	cfg := &types.Config{PodcastIndexConfig: types.PodcastIndexConfig{PodcastIndexAPIKeyFile: path}}

	key, secret := ResolvePodcastIndexCredentials(cfg)
	if key != "env-key" || secret != "env-secret" {
		t.Fatalf("environment should win: key=%q secret=%q", key, secret)
	}
}

func TestResolvePodcastIndexCredentialsDefaultsToAuthFolder(t *testing.T) {
	clearPodcastIndexEnv(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	dir := filepath.Join(home, ".config", "auth")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "podcast_index_key.json")
	if err := os.WriteFile(path, []byte(`{"api_key":"home-key","api_secret":"home-secret"}`), 0600); err != nil {
		t.Fatal(err)
	}

	key, secret := ResolvePodcastIndexCredentials(&types.Config{})
	if key != "home-key" || secret != "home-secret" {
		t.Fatalf("expected the auth-folder default to be read: key=%q secret=%q", key, secret)
	}
}

func TestResolvePodcastIndexCredentialsMissingIsEmpty(t *testing.T) {
	clearPodcastIndexEnv(t)
	cfg := &types.Config{PodcastIndexConfig: types.PodcastIndexConfig{
		PodcastIndexAPIKeyFile: filepath.Join(t.TempDir(), "absent.json"),
	}}
	key, secret := ResolvePodcastIndexCredentials(cfg)
	if key != "" || secret != "" {
		t.Fatalf("a missing file must yield no credentials, got key=%q secret=%q", key, secret)
	}
}
