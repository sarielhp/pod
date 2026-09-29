package podcast

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatsCategorisesFilesAndCountsPodcasts(t *testing.T) {
	lib, regular, favorite := pruneLibrary(t)
	if err := os.MkdirAll(filepath.Join(regular, ".work"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(regular, ".work", "tmp.wav"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(favorite, "cover.jpg"), []byte("123"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := lib.Stats(StatsOptions{Top: 5, PruneKeep: 5})
	if st.Podcasts != 2 || st.Favorites != 1 {
		t.Errorf("want 2 podcasts and 1 favorite, got %+v", st)
	}
	if st.Episodes != 14 || st.AdsRemoved != 14 || st.WithTranscript != 14 {
		t.Errorf("episode counts wrong: %+v", st)
	}
	if st.AudioBytes != 14*4 || st.OriginalBytes != 14*4 || st.WorkBytes != 5 || st.OtherBytes < 3 {
		t.Errorf("byte categories wrong: %+v", st)
	}
	sum := st.AudioBytes + st.OriginalBytes + st.TranscriptBytes + st.WorkBytes + st.OtherBytes
	if sum != st.TotalBytes {
		t.Errorf("categories should add up to the total: %d vs %d", sum, st.TotalBytes)
	}
	if len(st.Largest) != 2 || st.Largest[0].Bytes < st.Largest[1].Bytes {
		t.Errorf("largest podcasts should be listed biggest first: %+v", st.Largest)
	}
	if st.PrunableBytes != 2*2*4 {
		t.Errorf("only the regular podcast is prunable (2 episodes x audio+original), got %d", st.PrunableBytes)
	}
	if st.DiskTotal == 0 || st.DiskFree == 0 {
		t.Errorf("disk space should be reported, got total %d free %d", st.DiskTotal, st.DiskFree)
	}
}
