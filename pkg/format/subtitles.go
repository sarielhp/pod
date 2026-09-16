package format

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"pod/pkg/types"
	"pod/pkg/util"
)

func formatSRT(data *types.TranscriptionData) string {
	if data == nil {
		return ""
	}
	var lines []string
	for idx, seg := range data.Segments {
		st := FormatSRTTime(seg.Start)
		en := FormatSRTTime(seg.End)
		text := strings.TrimSpace(seg.Text)
		lines = append(lines, fmt.Sprintf("%d", idx+1))
		lines = append(lines, fmt.Sprintf("%s --> %s", st, en))
		lines = append(lines, text)
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n"
}

func ConvertJSONToSRT(inputFile string, data *types.TranscriptionData, customPath string, quiet bool) (string, error) {
	if data == nil {
		if !util.FileExists(inputFile) {
			return "", fmt.Errorf("cannot convert to SRT, JSON file not found: '%s'", inputFile)
		}
		raw, err := os.ReadFile(inputFile)
		if err != nil {
			return "", fmt.Errorf("error reading JSON file: %w", err)
		}
		var td types.TranscriptionData
		if err := json.Unmarshal(raw, &td); err != nil {
			return "", fmt.Errorf("error parsing JSON: %w", err)
		}
		data = &td
	}

	base := util.StripExt(inputFile)
	srtFile := customPath
	if srtFile == "" || !strings.HasSuffix(srtFile, ".srt") {
		srtFile = base + ".srt"
	}

	content := formatSRT(data)
	if err := util.WriteFileAtomic(srtFile, []byte(content), 0644); err != nil {
		return "", err
	}

	if !quiet {
		fmt.Printf("Converted and saved SubRip Subtitle file (.srt) to: '%s'\n", srtFile)
	}
	return srtFile, nil
}

func formatTXT(data *types.TranscriptionData, totalDuration float64, baseName string) string {
	if data == nil {
		return ""
	}
	lang := data.Language
	if lang == "" {
		lang = "auto"
	}

	var lines []string
	lines = append(lines, util.RepeatStr("=", 80))
	lines = append(lines, fmt.Sprintf("PODCAST TRANSCRIPTION: %s", util.FilepathBase(baseName)))
	lines = append(lines, fmt.Sprintf("Original Duration: %s (%.1fs) | Language: %s", FormatTime(totalDuration), totalDuration, strings.ToUpper(lang)))
	lines = append(lines, util.RepeatStr("=", 80))
	lines = append(lines, "")

	if len(data.Segments) == 0 && data.Text != "" {
		lines = append(lines, fmt.Sprintf("[00:00.0 -> %s] %s", FormatTime(totalDuration), data.Text))
	} else {
		for _, seg := range data.Segments {
			st := seg.Start
			en := seg.End
			text := strings.TrimSpace(seg.Text)
			lines = append(lines, fmt.Sprintf("[%s -> %s] %s", FormatTime(st), FormatTime(en), text))
			if len(seg.Words) > 0 {
				var wordStrs []string
				for _, w := range seg.Words {
					wordStrs = append(wordStrs, fmt.Sprintf("%s(%.2fs)", w.Word, w.Start))
				}
				lines = append(lines, "  Words: "+strings.Join(wordStrs, " "))
			}
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func ConvertJSONToTXT(inputFile string, data *types.TranscriptionData, totalDuration float64, customPath string, quiet bool) (string, error) {
	if data == nil {
		if !util.FileExists(inputFile) {
			return "", fmt.Errorf("cannot convert to TXT, JSON file not found: '%s'", inputFile)
		}
		raw, err := os.ReadFile(inputFile)
		if err != nil {
			return "", fmt.Errorf("error reading JSON file: %w", err)
		}
		var td types.TranscriptionData
		if err := json.Unmarshal(raw, &td); err != nil {
			return "", fmt.Errorf("error parsing JSON: %w", err)
		}
		data = &td
	}

	base := util.StripExt(inputFile)
	txtFile := customPath
	if txtFile == "" || !strings.HasSuffix(txtFile, ".txt") {
		txtFile = base + ".transcript.txt"
	}

	content := formatTXT(data, totalDuration, base)
	if err := util.WriteFileAtomic(txtFile, []byte(content), 0644); err != nil {
		return "", err
	}

	if !quiet {
		fmt.Printf("Converted and saved text transcript (.txt) to: '%s'\n", txtFile)
	}
	return txtFile, nil
}

func SaveJSONTranscript(mainFile string, data *types.TranscriptionData, jsonFile string, quiet bool, id3Tags map[string]string) error {
	outputData := make(map[string]interface{})
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &outputData); err != nil {
		return err
	}

	for k, v := range id3Tags {
		outputData["id3_"+k] = v
	}

	content, err := json.MarshalIndent(outputData, "", "  ")
	if err != nil {
		return err
	}
	if err := util.WriteFileAtomic(jsonFile, append(content, '\n'), 0644); err != nil {
		return err
	}
	if !quiet {
		fmt.Printf("Saved raw Whisper JSON data (.json) to: '%s'\n", jsonFile)
	}
	return nil
}
