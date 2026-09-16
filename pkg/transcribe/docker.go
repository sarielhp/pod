package transcribe

import (
	"fmt"
	"os/exec"
	"strings"

	"pod/pkg/util"
)

var dockerMu util.SyncMutex

type FailedToDecodeType struct{}

var FailedToDecodeSentinel = &FailedToDecodeType{}

func FetchDockerLogs(containerName string, tail int) string {
	if containerName == "" {
		return ""
	}
	dockerMu.Lock()
	defer dockerMu.Unlock()

	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", tail), containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(output)
}

func PollWhisperDockerProgress(containerName string) interface{} {
	if containerName == "" {
		return nil
	}

	logText := FetchDockerLogs(containerName, 20)
	if logText == "" {
		return nil
	}

	hasFailed := false
	for _, line := range util.SplitLines(logText) {
		if matchFailedDecode(line) {
			hasFailed = true
			continue
		}
		if h, m, s, ok := matchProgressHMS(line); ok {
			return float64(h*3600 + m*60 + s)
		}
		if m, s, ok := matchProgressMS(line); ok {
			return float64(m*60 + s)
		}
		if pct, ok := matchProgressPercent(line); ok {
			return pct / 100.0
		}
	}
	if hasFailed {
		return FailedToDecodeSentinel
	}
	return nil
}

func DetectWhisperDockerContainer(whisperURL string) string {
	host := util.ExtractHost(whisperURL)
	if !util.IsLocalHost(host) {
		return ""
	}

	dockerMu.Lock()
	cmd := exec.Command("docker", "ps", "--format", "{{.ID}}\t{{.Image}}\t{{.Names}}")
	output, err := cmd.Output()
	dockerMu.Unlock()
	if err != nil {
		return ""
	}

	type containerInfo struct {
		id    string
		image string
		name  string
	}
	var containers []containerInfo
	for _, line := range util.SplitLines(string(output)) {
		parts := util.SplitTab(line)
		if len(parts) >= 3 {
			containers = append(containers, containerInfo{id: parts[0], image: parts[1], name: parts[2]})
		}
	}

	for _, c := range containers {
		if strings.Contains(util.ToLower(c.image), "whisper") || strings.Contains(util.ToLower(c.name), "whisper") {
			return c.name
		}
	}
	return ""
}

func matchFailedDecode(line string) bool {
	lower := util.ToLower(line)
	return strings.Contains(lower, "failed to decode") || strings.Contains(lower, "failed to encode")
}

func matchProgressHMS(line string) (h, m, s int, ok bool) {
	_, err := fmt.Sscanf(line, "processing audio (%d:%d:%d", &h, &m, &s)
	if err == nil {
		return h, m, s, true
	}
	for i := 0; i < len(line)-10; i++ {
		if line[i:i+10] == "processing" {
			_, err := fmt.Sscanf(line[i:], "processing audio (%d:%d:%d", &h, &m, &s)
			if err == nil {
				return h, m, s, true
			}
			break
		}
	}
	return 0, 0, 0, false
}

func matchProgressMS(line string) (m, s int, ok bool) {
	for i := 0; i < len(line)-10; i++ {
		if line[i:i+10] == "processing" {
			_, err := fmt.Sscanf(line[i:], "processing audio (%d:%d", &m, &s)
			if err == nil {
				return m, s, true
			}
			break
		}
	}
	return 0, 0, false
}

func matchProgressPercent(line string) (float64, bool) {
	var pct float64
	_, err := fmt.Sscanf(line, "progress_common: %f%%", &pct)
	if err == nil {
		return pct, true
	}
	for i := 0; i < len(line)-10; i++ {
		if line[i:i+10] == "progress_c" {
			_, err := fmt.Sscanf(line[i:], "progress_common: %f%%", &pct)
			if err == nil {
				return pct, true
			}
			break
		}
	}
	return 0, false
}
