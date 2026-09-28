package podcast

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"pod/pkg/backend"
)

type orphanDeleteBackend struct {
	stubBackend
	failID string
}

func (b orphanDeleteBackend) Podcasts() ([]backend.Podcast, error) {
	mk := func(id, title string) backend.Podcast {
		var p backend.Podcast
		p.ID = id
		p.Media.Metadata.Title = title
		return p
	}
	return []backend.Podcast{mk("ok", "Deletable"), mk("bad", "Stuck")}, nil
}

func (b orphanDeleteBackend) DeletePodcast(id string) error {
	if id == b.failID {
		return errors.New("backend refused")
	}
	return nil
}

func TestRunCleanOrphansFailsWhenADeleteFails(t *testing.T) {
	var out bytes.Buffer
	res, err := RunCleanOrphans(orphanDeleteBackend{failID: "bad"}, CleanOrphansOptions{Force: true, Quiet: true, Out: &out})
	if res.DeletedCount != 1 || res.FailedCount != 1 {
		t.Fatalf("deleted %d failed %d, want 1 and 1", res.DeletedCount, res.FailedCount)
	}
	if err == nil {
		t.Fatal("RunCleanOrphans returned nil although one deletion failed")
	}
	if !strings.Contains(err.Error(), "Stuck") || !strings.Contains(err.Error(), "backend refused") {
		t.Fatalf("error does not name the failed podcast and cause: %v", err)
	}
}

func TestRunCleanOrphansSucceedsWhenEveryDeleteSucceeds(t *testing.T) {
	res, err := RunCleanOrphans(orphanDeleteBackend{}, CleanOrphansOptions{Force: true, Quiet: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.DeletedCount != 2 || res.FailedCount != 0 {
		t.Fatalf("deleted %d failed %d", res.DeletedCount, res.FailedCount)
	}
}
