package gemini

import "testing"

func TestNormaliseClockTimestamps(t *testing.T) {
	t.Parallel()

	t.Run("the real gemini-3.6-flash response shape", func(t *testing.T) {
		t.Parallel()
		// A bare 01:39 is not a JSON number, so the whole answer was
		// discarded over a formatting habit.
		in := `{"cuts":[{"start": 01:39,"end": 01:45,"type":"music_interlude"}]}`
		got := normaliseClockTimestamps(in)
		payload, err := ParseGeminiJSONString(got)
		if err != nil {
			t.Fatalf("still unparseable: %v", err)
		}
		if len(payload.Cuts) != 1 {
			t.Fatalf("cuts = %d", len(payload.Cuts))
		}
		if payload.Cuts[0].Start != 99 || payload.Cuts[0].End != 105 {
			t.Errorf("got %v..%v, want 99..105", payload.Cuts[0].Start, payload.Cuts[0].End)
		}
	})

	t.Run("hours are handled", func(t *testing.T) {
		t.Parallel()
		got := normaliseClockTimestamps(`{"start": 01:02:03}`)
		if got != `{"start": 3723}` {
			t.Errorf("got %s", got)
		}
	})

	t.Run("plain seconds are left alone", func(t *testing.T) {
		t.Parallel()
		in := `{"cuts":[{"start": 99.5,"end": 105}]}`
		if got := normaliseClockTimestamps(in); got != in {
			t.Errorf("rewrote valid input: %s", got)
		}
	})

	t.Run("text that merely looks like a clock is untouched", func(t *testing.T) {
		t.Parallel()
		// Only the start and end fields are rewritten, so a reason mentioning
		// a time keeps its wording.
		in := `{"reason":"ad at 01:39","start": 99}`
		if got := normaliseClockTimestamps(in); got != in {
			t.Errorf("rewrote a reason field: %s", got)
		}
	})
}

func TestParseGeminiJSONStringRecoversClockTimestamps(t *testing.T) {
	t.Parallel()
	raw := "```json\n{\"cuts\":[{\"start\": 03:04,\"end\": 03:13,\"reason\":\"music\"}],\"segments\":[]}\n```"
	payload, err := ParseGeminiJSONString(raw)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if payload.Cuts[0].Start != 184 || payload.Cuts[0].End != 193 {
		t.Errorf("got %v..%v, want 184..193", payload.Cuts[0].Start, payload.Cuts[0].End)
	}
}

func TestAudioMIMETypeMatchesTheUpload(t *testing.T) {
	t.Parallel()
	// The upload and the request must agree, or the API refuses with
	// "MIME type audio/mpeg does not match parent MIME type audio/wav".
	// Every chunk pod sends is WAV, and the request side said MP3.
	if got := AudioMIMEType("/tmp/x/.work/ep.mp3.wav"); got != "audio/wav" {
		t.Errorf("wav declared as %q", got)
	}
	if got := AudioMIMEType("/tmp/EP.WAV"); got != "audio/wav" {
		t.Errorf("uppercase extension declared as %q", got)
	}
	if got := AudioMIMEType("/tmp/ep.mp3"); got != "audio/mpeg" {
		t.Errorf("mp3 declared as %q", got)
	}
}
