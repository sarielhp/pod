package podcast

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"pod/pkg/backend"
)

type orphanTestBackend struct {
	backend.Backend
	podcasts []backend.Podcast
	deleted  []string
	failOn   map[string]error
}

func (b *orphanTestBackend) Podcasts() ([]backend.Podcast, error) { return b.podcasts, nil }

func (b *orphanTestBackend) DeletePodcast(id string) error {
	if err, ok := b.failOn[id]; ok {
		return err
	}
	b.deleted = append(b.deleted, id)
	return nil
}

func orphanFixture() *orphanTestBackend {
	mk := func(id, title, feed string, episodes int) backend.Podcast {
		p := backend.Podcast{ID: id}
		p.Media.Metadata.Title = title
		p.Media.Metadata.FeedURL = feed
		p.Media.Episodes = make([]backend.Episode, episodes)
		return p
	}
	return &orphanTestBackend{podcasts: []backend.Podcast{
		mk("keep", "Real Show", "https://example.com/real.xml", 5),
		mk("nofeed", "Fake Entry", "", 0),
		mk("dup", "Real Show (copy)", "https://example.com/real.xml/", 1),
	}}
}

func TestRunCleanOrphansDryRunDeletesNothing(t *testing.T) {
	t.Parallel()
	be := orphanFixture()
	var out bytes.Buffer
	res, err := RunCleanOrphans(be, CleanOrphansOptions{DryRun: true, Out: &out})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if res.ScannedCount != 3 || res.OrphanCount != 2 {
		t.Fatalf("scanned %d, orphans %d; want 3 and 2", res.ScannedCount, res.OrphanCount)
	}
	if res.DeletedCount != 0 || len(be.deleted) != 0 {
		t.Fatalf("dry run deleted %v", be.deleted)
	}
	if !strings.Contains(out.String(), "Dry run enabled") {
		t.Fatalf("dry run output did not say so:\n%s", out.String())
	}
}

func TestRunCleanOrphansDeclinedConfirmationDeletesNothing(t *testing.T) {
	t.Parallel()
	be := orphanFixture()
	res, err := RunCleanOrphans(be, CleanOrphansOptions{In: strings.NewReader("n\n")})
	if err != nil {
		t.Fatalf("declined confirmation: %v", err)
	}
	if res.DeletedCount != 0 || len(be.deleted) != 0 {
		t.Fatalf("declined confirmation still deleted %v", be.deleted)
	}
}

func TestRunCleanOrphansForceDeletesEveryOrphan(t *testing.T) {
	t.Parallel()
	be := orphanFixture()
	res, err := RunCleanOrphans(be, CleanOrphansOptions{Force: true, Quiet: true})
	if err != nil {
		t.Fatalf("force: %v", err)
	}
	if res.DeletedCount != 2 || res.FailedCount != 0 {
		t.Fatalf("deleted %d failed %d; want 2 and 0", res.DeletedCount, res.FailedCount)
	}
	got := strings.Join(be.deleted, ",")
	if !strings.Contains(got, "nofeed") || !strings.Contains(got, "dup") || strings.Contains(got, "keep") {
		t.Fatalf("deleted ids %v; want nofeed and dup only", be.deleted)
	}
}

func TestRunCleanOrphansCountsABackendFailure(t *testing.T) {
	t.Parallel()
	be := orphanFixture()
	boom := errors.New("boom")
	be.failOn = map[string]error{"nofeed": boom}
	var out bytes.Buffer
	res, err := RunCleanOrphans(be, CleanOrphansOptions{Force: true, Out: &out})
	if res.FailedCount != 1 || res.DeletedCount != 1 {
		t.Fatalf("failed %d deleted %d; want 1 and 1", res.FailedCount, res.DeletedCount)
	}
	if len(res.Errors) != 1 || !errors.Is(res.Errors[0], boom) {
		t.Fatalf("Errors = %v; want the backend's error", res.Errors)
	}
	if err != nil && !strings.Contains(err.Error(), "boom") {
		t.Fatalf("returned error does not mention the failure: %v", err)
	}
	if !strings.Contains(out.String(), "Failed to delete") {
		t.Fatalf("output does not report the failed delete:\n%s", out.String())
	}
}
