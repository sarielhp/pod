package episode

import (
	"os"
	"path/filepath"
	"testing"

	"pod/pkg/pipeline"
	"pod/pkg/types"
	"pod/pkg/util"
)

func TestResolveEpisodePaths(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	mp3Path := filepath.Join(tmp, "episode1.mp3")
	if err := os.WriteFile(mp3Path, []byte("fake mp3 audio"), 0644); err != nil {
		t.Fatal(err)
	}

	ep, err := Resolve(mp3Path)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if ep.Main != mp3Path {
		t.Errorf("ep.Main = %q, want %q", ep.Main, mp3Path)
	}
	if ep.Precut != mp3Path+".precut" {
		t.Errorf("ep.Precut = %q, want %q", ep.Precut, mp3Path+".precut")
	}
	if ep.Source != mp3Path {
		t.Errorf("ep.Source = %q, want %q", ep.Source, mp3Path)
	}
	if ep.Output != mp3Path {
		t.Errorf("ep.Output = %q, want %q", ep.Output, mp3Path)
	}
	wantTranscript := filepath.Join(tmp, "episode1.transcript.json")
	if ep.Transcript != wantTranscript {
		t.Errorf("ep.Transcript = %q, want %q", ep.Transcript, wantTranscript)
	}
	wantCuts := filepath.Join(tmp, "episode1.cuts.json")
	if ep.Cuts != wantCuts {
		t.Errorf("ep.Cuts = %q, want %q", ep.Cuts, wantCuts)
	}
	wantWorkDir := util.WorkDirFor(mp3Path)
	if ep.WorkDir != wantWorkDir {
		t.Errorf("ep.WorkDir = %q, want %q", ep.WorkDir, wantWorkDir)
	}
	if ep.Dir != tmp {
		t.Errorf("ep.Dir = %q, want %q", ep.Dir, tmp)
	}
}

func TestResolveEpisodeWithPrecutSource(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	mp3Path := filepath.Join(tmp, "show.mp3")
	precutPath := mp3Path + ".precut"
	if err := os.WriteFile(mp3Path, []byte("cut audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(precutPath, []byte("original precut audio"), 0644); err != nil {
		t.Fatal(err)
	}

	ep, err := Resolve(mp3Path)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if ep.Source != precutPath {
		t.Errorf("ep.Source = %q, want precut %q", ep.Source, precutPath)
	}

	epFromPrecut, err := Resolve(precutPath)
	if err != nil {
		t.Fatalf("Resolve precut failed: %v", err)
	}
	if epFromPrecut.Main != mp3Path {
		t.Errorf("epFromPrecut.Main = %q, want %q", epFromPrecut.Main, mp3Path)
	}
}

func TestResolveEmptyPath(t *testing.T) {
	t.Parallel()

	_, err := Resolve("   ")
	if err == nil {
		t.Error("expected error resolving empty path")
	}
}

func TestEpisodeStatusHelpers(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	mp3Path := filepath.Join(tmp, "test_status.mp3")
	if err := os.WriteFile(mp3Path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	ep, err := Resolve(mp3Path)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if ep.IsClean() {
		t.Error("ep.IsClean() should be false before transcript/clean status")
	}

	stInit := &types.EpisodeStatusFile{
		Status: types.StateDownloaded,
	}
	if err := pipeline.SaveEpisodeStatus(pipeline.StatusPathFor(ep.Main), stInit); err != nil {
		t.Fatal(err)
	}

	st, err := ep.Status()
	if err != nil {
		t.Fatalf("ep.Status() failed: %v", err)
	}
	if st.Status != types.StateDownloaded {
		t.Errorf("st.Status = %v, want StateDownloaded", st.Status)
	}

	err = ep.Update(func(s *types.EpisodeStatusFile) {
		s.Status = types.StateDone
	})
	if err != nil {
		t.Fatalf("ep.Update() failed: %v", err)
	}

	stUpdated, err := ep.Status()
	if err != nil {
		t.Fatalf("ep.Status() after update failed: %v", err)
	}
	if stUpdated.Status != types.StateDone {
		t.Errorf("stUpdated.Status = %v, want StateDone", stUpdated.Status)
	}
}

func TestDetectPodcastDirForAudio(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	showDir := filepath.Join(tmp, "MyShow")
	epDir := filepath.Join(showDir, "Episode1")
	if err := os.MkdirAll(epDir, 0755); err != nil {
		t.Fatal(err)
	}

	podAudio := filepath.Join(epDir, "podcast.mp3")
	if err := os.WriteFile(podAudio, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	got := DetectPodcastDirForAudio(podAudio)
	if got != showDir {
		t.Errorf("got %q, want %q", got, showDir)
	}

	regularAudio := filepath.Join(showDir, "ep.mp3")
	gotRegular := DetectPodcastDirForAudio(regularAudio)
	if gotRegular != showDir {
		t.Errorf("got %q, want %q", gotRegular, showDir)
	}
}
