package transcribe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pod/pkg/types"
)

func withUsableProfiles(t *testing.T) {
	old := WhisperProfileUsable
	WhisperProfileUsable = func(wp types.WhisperProfile) bool { return wp.URL != "" }
	t.Cleanup(func() { WhisperProfileUsable = old })
}

func cfgWith(p []types.WhisperProfile) types.Config {
	var c types.Config
	c.WhisperProfiles = p
	return c
}

func TestSpeedRoutingNeverPicksADiarizingProfile(t *testing.T) {
	withUsableProfiles(t)
	cfg := cfgWith([]types.WhisperProfile{
		{ID: 1, Name: "plain", Engine: types.WhisperEngineRemote, URL: "http://plain", SpeedFactor: 5},
		{ID: 2, Name: "x", Engine: types.WhisperEngineRemote, URL: "http://x", SpeedFactor: 50, Diarize: true},
	})
	if got := ResolveWhisperProfileForLanguage(cfg, "en"); got.Name != "plain" {
		t.Fatalf("speed routing chose %q", got.Name)
	}
}

func TestDiarizingProfilePrefersTheOneNamingTheLanguage(t *testing.T) {
	withUsableProfiles(t)
	cfg := cfgWith([]types.WhisperProfile{
		{ID: 1, Name: "any", URL: "http://any", Diarize: true},
		{ID: 2, Name: "hebrew", URL: "http://he", Diarize: true, Languages: []string{"he"}},
		{ID: 3, Name: "plain", URL: "http://plain"},
	})
	if got, ok := ResolveDiarizingProfile(cfg, "he"); !ok || got.Name != "hebrew" {
		t.Fatalf("he: %v %v", got.Name, ok)
	}
	if got, ok := ResolveDiarizingProfile(cfg, "en"); !ok || got.Name != "any" {
		t.Fatalf("en: %v %v", got.Name, ok)
	}
	if _, ok := ResolveDiarizingProfile(types.Config{}, "en"); ok {
		t.Fatal("no profile configured must report none")
	}
}

func TestModelForLanguageOverridesTheDefault(t *testing.T) {
	wp := types.WhisperProfile{Model: "large-v3-turbo", ModelByLanguage: map[string]string{"he": "ivrit"}}
	if ModelForLanguage(wp, "HE") != "ivrit" || ModelForLanguage(wp, "en") != "large-v3-turbo" || ModelForLanguage(wp, "") != "large-v3-turbo" {
		t.Fatal("wrong model")
	}
}

func TestWhisperRequestSendsExtraFieldsAndReadsSpeakers(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		got = map[string]string{}
		for k, v := range r.MultipartForm.Value {
			got[k] = v[0]
		}
		_ = json.NewEncoder(w).Encode(types.TranscriptionData{
			Text: "hi", Diarized: true, Speakers: []string{"SPEAKER_00"},
			Segments: []types.TranscriptionSegment{{Start: 0, End: 1, Text: "hi", Speaker: "SPEAKER_00"}},
		})
	}))
	defer srv.Close()
	audio := filepath.Join(t.TempDir(), "a.wav")
	if err := os.WriteFile(audio, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	td, err := TranscribeWhisperRequest(context.Background(), WhisperRequest{
		AudioPath: audio, URL: srv.URL, Quiet: true, TotalDuration: 1, Language: "he",
		Fields: map[string]string{"diarize": "true", "model": "ivrit"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["diarize"] != "true" || got["model"] != "ivrit" || got["language"] != "he" {
		t.Fatalf("fields sent: %v", got)
	}
	if !td.Diarized || td.Segments[0].Speaker != "SPEAKER_00" || len(td.Speakers) != 1 {
		t.Fatalf("speakers lost: %+v", td)
	}
}

func TestMessagesNameTheServerThatWasAsked(t *testing.T) {
	oldUnit := retryUnit
	retryUnit = time.Millisecond
	t.Cleanup(func() { retryUnit = oldUnit })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()
	audio := filepath.Join(t.TempDir(), "a.wav")
	if err := os.WriteFile(audio, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"": "connect to Whisper GPU server", "WhisperX": "connect to WhisperX GPU server"} {
		_, err := TranscribeWhisperRequest(context.Background(), WhisperRequest{AudioPath: audio, URL: srv.URL, Quiet: true, TotalDuration: 1, ServerName: name})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ServerName %q: err = %v, want it to contain %q", name, err, want)
		}
	}
}
