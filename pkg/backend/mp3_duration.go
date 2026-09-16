package backend

import (
	"io"
	"os"

	"github.com/tcolgate/mp3"
)

func getMP3DiskDurationNative(path string) float64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	d := mp3.NewDecoder(file)
	var duration float64
	var frame mp3.Frame
	var skipped int

	for {
		if err := d.Decode(&frame, &skipped); err != nil {
			if err == io.EOF {
				return duration
			}
			return 0
		}
		duration += frame.Duration().Seconds()
	}
}
