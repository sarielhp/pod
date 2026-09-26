package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"pod/pkg/types"
)

type mockTranscriber struct {
	td    *types.TranscriptionData
	err   error
	delay time.Duration
}

func (m *mockTranscriber) Transcribe(ctx context.Context, wav string, duration float64) (*types.TranscriptionData, error) {
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.delay):
		}
	}
	return m.td, m.err
}

func TestFallbackTranscriberPrimarySuccess(t *testing.T) {
	t.Parallel()

	expected := &types.TranscriptionData{Text: "primary success"}
	primary := &mockTranscriber{td: expected}
	backup := &mockTranscriber{err: errors.New("backup should not run")}

	fb := NewFallbackTranscriber(primary, backup, nil)
	got, err := fb.Transcribe(context.Background(), "test.wav", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Text != expected.Text {
		t.Errorf("got %q, want %q", got.Text, expected.Text)
	}
}

func TestFallbackTranscriberFallsBackOnFailure(t *testing.T) {
	t.Parallel()

	primary := &mockTranscriber{err: errors.New("primary error")}
	expected := &types.TranscriptionData{Text: "backup success"}
	backup := &mockTranscriber{td: expected}

	fb := NewFallbackTranscriber(primary, backup, nil)
	got, err := fb.Transcribe(context.Background(), "test.wav", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Text != expected.Text {
		t.Errorf("got %q, want %q", got.Text, expected.Text)
	}
}

func TestFallbackTranscriberContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	primary := &mockTranscriber{err: errors.New("primary failed")}
	backup := &mockTranscriber{td: &types.TranscriptionData{Text: "backup"}}

	fb := NewFallbackTranscriber(primary, backup, nil)
	_, err := fb.Transcribe(ctx, "test.wav", 10)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRacingTranscriberFastestWins(t *testing.T) {
	t.Parallel()

	slow := &mockTranscriber{
		td:    &types.TranscriptionData{Text: "slow"},
		delay: 100 * time.Millisecond,
	}
	fast := &mockTranscriber{
		td:    &types.TranscriptionData{Text: "fast"},
		delay: 10 * time.Millisecond,
	}

	racers := []NamedTranscriber{
		{Name: "slow", Transcriber: slow},
		{Name: "fast", Transcriber: fast},
	}

	rt := NewRacingTranscriber(racers, nil)
	got, err := rt.Transcribe(context.Background(), "test.wav", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Text != "fast" {
		t.Errorf("got %q, want fast", got.Text)
	}
}

func TestRacingTranscriberAllFail(t *testing.T) {
	t.Parallel()

	r1 := &mockTranscriber{err: errors.New("err1")}
	r2 := &mockTranscriber{err: errors.New("err2")}

	racers := []NamedTranscriber{
		{Name: "r1", Transcriber: r1},
		{Name: "r2", Transcriber: r2},
	}

	rt := NewRacingTranscriber(racers, nil)
	_, err := rt.Transcribe(context.Background(), "test.wav", 10)
	if err == nil {
		t.Fatal("expected error when all racers fail")
	}
}

func TestRacingTranscriberSingleRacer(t *testing.T) {
	t.Parallel()

	expected := &types.TranscriptionData{Text: "solo"}
	solo := &mockTranscriber{td: expected}

	rt := NewRacingTranscriber([]NamedTranscriber{{Name: "solo", Transcriber: solo}}, nil)
	got, err := rt.Transcribe(context.Background(), "test.wav", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Text != expected.Text {
		t.Errorf("got %q, want %q", got.Text, expected.Text)
	}
}

func TestRacingTranscriberEmpty(t *testing.T) {
	t.Parallel()

	rt := NewRacingTranscriber(nil, nil)
	_, err := rt.Transcribe(context.Background(), "test.wav", 10)
	if err == nil {
		t.Fatal("expected error for empty racers")
	}
}
