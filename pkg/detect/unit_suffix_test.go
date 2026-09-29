package detect

import "testing"

func TestExtractJSONArrayAcceptsTimesWrittenWithAnSSuffix(t *testing.T) {
	ads, err := ExtractJSONArray(`[{"start": 0.0, "end": 33.8s, "reason": "a"}, {"start": 33.8s, "end": 62.6s, "reason": "b, 5s"}]`)
	if err != nil || len(ads) != 2 || ads[1].Start != 33.8 || ads[1].End != 62.6 || ads[1].Reason != "b, 5s" {
		t.Fatalf("ads = %+v, err = %v", ads, err)
	}
	if _, err := ExtractJSONArray(`[{"start": oops}]`); err == nil {
		t.Fatal("a genuinely broken reply must still be an error")
	}
}
