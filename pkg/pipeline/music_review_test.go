package pipeline

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/music"
	"pod/pkg/types"
)

func reviewFixture(t *testing.T, duration float64) (*MusicReview, *music.Result) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "episode.mp3")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	result := &music.Result{Duration: duration}
	for sec := 0.0; sec < duration; sec += 0.48 {
		result.Frames = append(result.Frames, music.Frame{Time: sec, Music: 0.98})
	}
	review := &MusicReview{req: MusicReviewRequest{AudioPath: path, Duration: duration}, result: make(chan classification, 1)}
	review.result <- classification{result: result}
	review.retry = func(context.Context, types.KeepSegment) (*types.TranscriptionData, error) {
		return &types.TranscriptionData{}, nil
	}
	return review, result
}

func TestMusicReviewRecoversAdThenCutsOnlyResidualMusic(t *testing.T) {
	review, result := reviewFixture(t, 20)
	td := &types.TranscriptionData{Segments: []types.TranscriptionSegment{{Start: 0, End: 2, Text: "intro"}, {Start: 18, End: 20, Text: "episode"}}}
	for i := range result.Frames {
		if result.Frames[i].Time < 7 {
			result.Frames[i].Speech = 0.9
		}
	}
	calls := 0
	review.retry = func(_ context.Context, gap types.KeepSegment) (*types.TranscriptionData, error) {
		calls++
		if gap.Start != 2 || gap.End != 18 {
			t.Fatalf("unexpected gap: %+v", gap)
		}
		return &types.TranscriptionData{Segments: []types.TranscriptionSegment{{Start: 2, End: 8, Text: "if you like YouTube you would love YouTube Premium", Words: []types.TranscriptionWord{{Start: 2, End: 8, Word: "YouTube Premium advertisement"}}}}}, nil
	}
	if err := review.Review(context.Background(), td); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(td.Text, "YouTube Premium") || !td.GapReview.Complete || len(td.Cuts) != 1 {
		t.Fatalf("unexpected review: %+v %+v", td, td.GapReview)
	}
	if td.Cuts[0].Start < 8.75 || td.Cuts[0].End > 17.25 {
		t.Fatalf("music cut touches speech: %+v", td.Cuts)
	}
	review.result <- classification{result: result}
	review.retry = func(context.Context, types.KeepSegment) (*types.TranscriptionData, error) {
		t.Fatal("completed cached review retried Whisper")
		return nil, nil
	}
	if err := review.Review(context.Background(), td); err != nil {
		t.Fatal(err)
	}
}

func TestMusicReviewVetoSilenceUnknownAndFailure(t *testing.T) {
	cases := []struct {
		name                   string
		speech, silence, music float64
		fails, cuts            bool
	}{
		{"music", 0, 0, 0.99, false, true},
		{"speech over music", 0.2, 0, 0.99, false, false},
		{"weak speech veto", 0.07, 0, 0.99, false, false},
		{"silence", 0, 0.99, 0, false, false},
		{"unknown", 0, 0, 0.2, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, result := reviewFixture(t, 12)
			for i := range result.Frames {
				result.Frames[i].Speech = tc.speech
				result.Frames[i].Silence = tc.silence
				result.Frames[i].Music = tc.music
			}
			td := &types.TranscriptionData{Segments: []types.TranscriptionSegment{{Start: 0, End: 2, Text: "start"}, {Start: 10, End: 12, Text: "end"}}}
			err := r.Review(context.Background(), td)
			if (err != nil) != tc.fails || (len(td.Cuts) > 0) != tc.cuts {
				t.Fatalf("err=%v cuts=%+v", err, td.Cuts)
			}
		})
	}
	r, _ := reviewFixture(t, 12)
	r.retry = func(context.Context, types.KeepSegment) (*types.TranscriptionData, error) {
		return nil, errors.New("offline")
	}
	td := &types.TranscriptionData{}
	if err := r.Review(context.Background(), td); err != nil || td.GapReview.Complete || len(td.Cuts) != 0 || len(td.GapReview.Unresolved) != 1 {
		t.Fatalf("failed review cut audio: %+v", td)
	}
}

func TestMusicReviewUsesWordGapsAndLimitsRetries(t *testing.T) {
	td := &types.TranscriptionData{Segments: []types.TranscriptionSegment{{Start: 0, End: 60, Text: "start end", Words: []types.TranscriptionWord{{Start: 0, End: 2, Word: "start"}, {Start: 58, End: 60, Word: "end"}}}}}
	gaps := transcriptGaps(td, 60, 0)
	if len(gaps) != 1 || gaps[0].Start != 2 || gaps[0].End != 58 {
		t.Fatal(gaps)
	}
	r, _ := reviewFixture(t, 1200)
	calls := 0
	r.retry = func(context.Context, types.KeepSegment) (*types.TranscriptionData, error) {
		calls++
		return &types.TranscriptionData{}, nil
	}
	td = &types.TranscriptionData{}
	if err := r.Review(context.Background(), td); err != nil || calls != 30 || td.GapReview.Complete || len(td.GapReview.Unresolved) != 1 {
		t.Fatalf("retry bound failed: calls=%d err=%v", calls, err)
	}
}

func TestMusicReviewPreservesFailedGapAndRecoversLaterGap(t *testing.T) {
	cases := []struct {
		name string
		td   *types.TranscriptionData
		err  error
	}{
		{"offline", nil, errors.New("offline")},
		{"timeout", nil, context.DeadlineExceeded},
		{"missing transcript", nil, nil},
		{"empty transcript", &types.TranscriptionData{}, nil},
		{"untimed speech", &types.TranscriptionData{Text: "speech"}, nil},
		{"invalid timestamps", &types.TranscriptionData{Segments: []types.TranscriptionSegment{{Start: 4, End: 3, Text: "speech"}}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, result := reviewFixture(t, 22)
			for i := range result.Frames {
				if result.Frames[i].Time < 10 {
					result.Frames[i].Speech = 0.9
				}
			}
			td := &types.TranscriptionData{Segments: []types.TranscriptionSegment{{Start: 0, End: 2, Text: "intro"}, {Start: 10, End: 12, Text: "middle"}, {Start: 20, End: 22, Text: "end"}}, Cuts: []types.AdSegment{{Start: 0, End: 22, Reason: "old ad"}}}
			calls := 0
			r.retry = func(_ context.Context, gap types.KeepSegment) (*types.TranscriptionData, error) {
				calls++
				if calls == 1 {
					return tc.td, tc.err
				}
				return &types.TranscriptionData{Segments: []types.TranscriptionSegment{{Start: gap.Start, End: gap.End, Text: "recovered speech"}}}, nil
			}
			if err := r.Review(context.Background(), td); err != nil {
				t.Fatal(err)
			}
			if calls != 2 || td.GapReview.RecoveredSegments != 1 || td.GapReview.Complete {
				t.Fatalf("calls=%d review=%+v", calls, td.GapReview)
			}
			gaps := td.GapReview.Unresolved
			if len(gaps) != 1 || gaps[0].Start != 2 || gaps[0].End != 10 {
				t.Fatalf("unresolved gap changed: %+v", gaps)
			}
			for _, cut := range td.Cuts {
				if cut.Start < 10 && cut.End > 2 {
					t.Fatalf("cut removed preserved gap: %+v", cut)
				}
			}
		})
	}
}

func TestMusicReviewFailedRetriesRemainBoundedAndParentCancellationFails(t *testing.T) {
	r, _ := reviewFixture(t, 1200)
	calls := 0
	r.retry = func(context.Context, types.KeepSegment) (*types.TranscriptionData, error) {
		calls++
		return nil, errors.New("offline")
	}
	td := &types.TranscriptionData{}
	if err := r.Review(context.Background(), td); err != nil || calls != 30 || len(td.Cuts) != 0 || len(td.GapReview.Unresolved) != 1 {
		t.Fatalf("calls=%d err=%v review=%+v", calls, err, td.GapReview)
	}
	r, _ = reviewFixture(t, 12)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.retry = func(context.Context, types.KeepSegment) (*types.TranscriptionData, error) {
		cancel()
		return nil, context.DeadlineExceeded
	}
	if err := r.Review(ctx, &types.TranscriptionData{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("parent cancellation was ignored: %v", err)
	}
}

func TestMusicReviewStartsBeforeTranscriptAndCancels(t *testing.T) {
	started := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { started <- struct{}{}; <-req.Context().Done() }))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "original.mp3")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := StartMusicReview(ctx, MusicReviewRequest{AudioPath: path, CachePath: path + ".json", Config: types.Config{WhisperConfig: types.WhisperConfig{YamnetURL: srv.URL}}})
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("classifier waited for transcript")
	}
	cancel()
	if err := r.Review(ctx, &types.TranscriptionData{}); err == nil {
		t.Fatal("cancelled review succeeded")
	}
}

func TestMusicCutsUseOverlappingFrameSpeechVeto(t *testing.T) {
	result := &music.Result{Duration: 10}
	for sec := 0.0; sec < 10; sec += 0.48 {
		result.Frames = append(result.Frames, music.Frame{Time: sec, Music: 0.99})
	}
	result.Frames[8].Speech = 0.2
	for _, cut := range confirmedMusic(result, types.KeepSegment{Start: 0, End: 10}) {
		if math.Min(cut.End, 8*0.48+0.96) > math.Max(cut.Start, 8*0.48) {
			t.Fatalf("cut overlaps speech window: %+v", cut)
		}
	}
}

func TestMusicReviewRejectsSpeechWithoutTimestamps(t *testing.T) {
	r, _ := reviewFixture(t, 12)
	r.retry = func(context.Context, types.KeepSegment) (*types.TranscriptionData, error) {
		return &types.TranscriptionData{Text: "spoken advertisement"}, nil
	}
	td := &types.TranscriptionData{}
	if err := r.Review(context.Background(), td); err != nil || td.GapReview.Complete || len(td.Cuts) > 0 || len(td.GapReview.Unresolved) != 1 {
		t.Fatalf("untimed speech was treated as empty music: %+v", td)
	}
	seg := types.TranscriptionSegment{Start: 0, End: 12, Text: "spoken content", Words: []types.TranscriptionWord{{Start: 0, End: 0, Word: "spoken"}}}
	if gaps := transcriptGaps(&types.TranscriptionData{Segments: []types.TranscriptionSegment{seg}}, 12, 0); len(gaps) != 0 {
		t.Fatalf("invalid word timestamps discarded spoken segment: %+v", gaps)
	}
}

func TestMusicReviewAcceptsSmallDurationDrift(t *testing.T) {
	r, result := reviewFixture(t, 930)
	result.Duration = 932.13
	td := &types.TranscriptionData{Segments: []types.TranscriptionSegment{{Start: 0, End: 932.13, Text: "content"}}}
	if err := r.Review(context.Background(), td); err != nil {
		t.Fatalf("small duration drift rejected: %v", err)
	}
	if td.GapReview == nil || td.GapReview.Duration != 932.13 {
		t.Fatalf("did not adopt classified duration: %+v", td.GapReview)
	}
}

func TestMusicReviewRejectsGrossDurationMismatch(t *testing.T) {
	r, result := reviewFixture(t, 900)
	result.Duration = 600
	td := &types.TranscriptionData{}
	if err := r.Review(context.Background(), td); err == nil || !strings.Contains(err.Error(), "classifier duration does not match") {
		t.Fatalf("gross duration mismatch accepted: %v", err)
	}
}
