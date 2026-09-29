package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"pod/pkg/transcribe"
	"pod/pkg/types"
)

type recordedRequest struct{ model, language string }

func speakersServer(t *testing.T, replyLang string) (*httptest.Server, *[]recordedRequest) {
	var mu sync.Mutex
	var seen []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		mu.Lock()
		seen = append(seen, recordedRequest{r.FormValue("model"), r.FormValue("language")})
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(types.TranscriptionData{Language: replyLang, Text: "x", Diarized: true})
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func speakersConfig(url string) types.Config {
	var cfg types.Config
	cfg.WhisperProfiles = []types.WhisperProfile{{
		ID: 1, Name: "x", URL: url, Engine: types.WhisperEngineRemote, Diarize: true,
		Model: "general", ModelByLanguage: map[string]string{"he": "ivrit"},
	}}
	return cfg
}

func usableWhenItHasAURL(t *testing.T) {
	old := transcribe.WhisperProfileUsable
	transcribe.WhisperProfileUsable = func(wp types.WhisperProfile) bool { return wp.URL != "" }
	t.Cleanup(func() { transcribe.WhisperProfileUsable = old })
}

func speakersAudio(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "a.wav")
	if err := os.WriteFile(p, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSpeakersRedoesTheWorkWithTheLanguageModelWhenTheServerDetectsOne(t *testing.T) {
	usableWhenItHasAURL(t)
	srv, seen := speakersServer(t, "he")
	td, err := transcribeWithSpeakers(speakersAudio(t), speakersConfig(srv.URL), types.ProcOptions{Quiet: true}, 1, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []recordedRequest{{"general", ""}, {"ivrit", "he"}}
	if len(*seen) != 2 || (*seen)[0] != want[0] || (*seen)[1] != want[1] || td.Model != "ivrit" {
		t.Fatalf("requests %v, stamped model %q", *seen, td.Model)
	}
}

func TestSpeakersAsksOnceWhenTheLanguageIsKnownOrHasNoModelOfItsOwn(t *testing.T) {
	usableWhenItHasAURL(t)
	srv, seen := speakersServer(t, "en")
	if _, err := transcribeWithSpeakers(speakersAudio(t), speakersConfig(srv.URL), types.ProcOptions{Quiet: true}, 1, "", nil); err != nil || len(*seen) != 1 {
		t.Fatalf("detected English: %v requests, err %v", len(*seen), err)
	}
	srv2, seen2 := speakersServer(t, "he")
	if _, err := transcribeWithSpeakers(speakersAudio(t), speakersConfig(srv2.URL), types.ProcOptions{Quiet: true, Language: "he"}, 1, "", nil); err != nil || len(*seen2) != 1 || (*seen2)[0].model != "ivrit" {
		t.Fatalf("language given: %v, err %v", *seen2, err)
	}
}
