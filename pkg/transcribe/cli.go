package transcribe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"pod/pkg/types"
	"pod/pkg/util"
)

// extraWhisperModelDirs is set from configuration at startup and read on every
// transcription. The guard is there because tests set it too, and a test that
// does so runs alongside parallel tests that transcribe.
var (
	extraWhisperModelDirsMu util.Mutex
	extraWhisperModelDirs   []string
)

func SetExtraWhisperModelDirs(dirs []string) {
	extraWhisperModelDirsMu.Lock()
	defer extraWhisperModelDirsMu.Unlock()
	extraWhisperModelDirs = append([]string(nil), dirs...)
}

func extraWhisperModelDirsValue() []string {
	extraWhisperModelDirsMu.Lock()
	defer extraWhisperModelDirsMu.Unlock()
	return append([]string(nil), extraWhisperModelDirs...)
}

func GetWhisperModelSearchDirs() []string {
	var dirs []string
	if extra := extraWhisperModelDirsValue(); len(extra) > 0 {
		dirs = append(dirs, extra...)
	}
	dirs = append(dirs, "/media/dockers/whisper/models")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "share", "whisper-models"))
	}
	return dirs
}

func WhisperModelCandidates(model string) []string {
	norm := strings.ToLower(strings.TrimSpace(model))
	switch norm {
	case "tiny":
		return []string{"ggml-tiny.bin", "ggml-tiny.en.bin", "tiny.bin", "tiny.en.bin"}
	case "tiny.en":
		return []string{"ggml-tiny.en.bin", "tiny.en.bin", "ggml-tiny.bin"}
	case "base":
		return []string{"ggml-base.bin", "ggml-base.en.bin", "base.bin", "base.en.bin"}
	case "base.en":
		return []string{"ggml-base.en.bin", "base.en.bin", "ggml-base.bin"}
	case "distil":
		return []string{"ggml-distil-large-v3.bin", "distil-large-v3.bin", "ggml-distil-medium.en.bin"}
	case "ivrit":
		return []string{"ggml-ivrit-large-v3-turbo.bin", "ivrit-large-v3-turbo.bin"}
	case "turbo":
		return []string{"ggml-large-v3-turbo.bin", "large-v3-turbo.bin"}
	default:
		candidates := []string{norm}
		if !strings.HasPrefix(norm, "ggml-") {
			candidates = append(candidates, "ggml-"+norm+".bin", "ggml-"+norm)
		}
		if !strings.HasSuffix(norm, ".bin") {
			candidates = append(candidates, norm+".bin")
		}
		return candidates
	}
}

func ResolveWhisperModelPath(model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "tiny.en"
	}
	if util.FileExists(model) {
		return model, nil
	}
	dirs := GetWhisperModelSearchDirs()
	candidates := WhisperModelCandidates(model)
	for _, dir := range dirs {
		for _, c := range candidates {
			p := filepath.Join(dir, c)
			if util.FileExists(p) {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("whisper model %q not found in search paths (%s)", model, strings.Join(dirs, ", "))
}

func BuildWhisperCLIArgs(audioPath, modelPath, outJsonBase, lang, prompt string, processors, threads int, greedy bool) []string {
	if processors <= 0 {
		processors = 4
	}
	if threads <= 0 {
		threads = 4
	}
	args := []string{
		"-m", modelPath,
		"-f", audioPath,
		"-dev", "0",
		"-fa",
		"-p", strconv.Itoa(processors),
		"-t", strconv.Itoa(threads),
		"-oj",
		"-of", outJsonBase,
		"-np",
	}
	if greedy {
		args = append(args, "-bs", "1", "-bo", "1", "-nf")
	}
	if lang != "" {
		args = append(args, "-l", lang)
	}
	if prompt != "" {
		args = append(args, "--prompt", prompt)
	}
	return args
}

type whisperCLIJSON struct {
	Result struct {
		Language string `json:"language"`
	} `json:"result"`
	Transcription []whisperCLISegment `json:"transcription"`
}

type whisperCLISegment struct {
	Timestamps struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"timestamps"`
	Offsets struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	} `json:"offsets"`
	Text   string            `json:"text"`
	Tokens []whisperCLIToken `json:"tokens"`
}

type whisperCLIToken struct {
	Text    string `json:"text"`
	Offsets struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	} `json:"offsets"`
}

func ParseWhisperTimestamp(ts string) float64 {
	ts = strings.TrimSpace(strings.ReplaceAll(ts, ",", "."))
	parts := strings.Split(ts, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	s, _ := strconv.ParseFloat(parts[2], 64)
	return h*3600 + m*60 + s
}

func BuildSegmentWords(tokens []whisperCLIToken) []types.TranscriptionWord {
	var words []types.TranscriptionWord
	for _, tok := range tokens {
		wText := strings.TrimSpace(tok.Text)
		if wText != "" {
			words = append(words, types.TranscriptionWord{
				Start: float64(tok.Offsets.From) / 1000.0,
				End:   float64(tok.Offsets.To) / 1000.0,
				Word:  wText,
			})
		}
	}
	return words
}

func ParseWhisperCLIJSON(data []byte) (*types.TranscriptionData, error) {
	var out whisperCLIJSON
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("failed to parse whisper-cli JSON: %w", err)
	}
	var segments []types.TranscriptionSegment
	var textParts []string
	lang := out.Result.Language
	for _, seg := range out.Transcription {
		start := float64(seg.Offsets.From) / 1000.0
		end := float64(seg.Offsets.To) / 1000.0
		if start == 0 && end == 0 && seg.Timestamps.To != "" {
			start = ParseWhisperTimestamp(seg.Timestamps.From)
			end = ParseWhisperTimestamp(seg.Timestamps.To)
		}
		words := BuildSegmentWords(seg.Tokens)
		segText := strings.TrimSpace(seg.Text)
		segments = append(segments, types.TranscriptionSegment{
			Start:    start,
			End:      end,
			Text:     segText,
			Language: lang,
			Words:    words,
		})
		if segText != "" {
			textParts = append(textParts, segText)
		}
	}
	return &types.TranscriptionData{
		Text:     strings.Join(textParts, " "),
		Segments: segments,
		Language: lang,
	}, nil
}

func ResolveWhisperCLIBinary(bin string) string {
	if bin == "" {
		bin = "whisper-cli"
	}
	if p, err := exec.LookPath(bin); err == nil {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidate := filepath.Join(home, ".local", "bin", bin)
		if util.FileExists(candidate) {
			return candidate
		}
	}
	return bin
}

func RunWhisperCLITranscription(audioPath string, profile types.WhisperProfile, quiet, verbose bool, prompt, lang string) (*types.TranscriptionData, error) {
	return RunWhisperCLITranscriptionContext(context.Background(), audioPath, profile, quiet, verbose, prompt, lang)
}

func RunWhisperCLITranscriptionContext(ctx context.Context, audioPath string, profile types.WhisperProfile, quiet, verbose bool, prompt, lang string) (*types.TranscriptionData, error) {
	bin := ResolveWhisperCLIBinary(profile.CliBinary)
	AnnounceUsing(types.WhisperEngineLocal, fmt.Sprintf("(%s, model: %s)", bin, profile.Model), terminalReporter(quiet, verbose))
	modelPath, err := ResolveWhisperModelPath(profile.Model)
	if err != nil {
		return nil, err
	}
	workDir := util.WorkDirFor(audioPath)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp work directory: %w", err)
	}
	outJsonBase := filepath.Join(workDir, "whisper_cli_out")
	outJsonFile := outJsonBase + ".json"
	if err := util.VerifyTempFile(outJsonFile); err != nil {
		return nil, err
	}
	defer os.Remove(outJsonFile)

	if lang == "" {
		lang = profile.Language
	}
	if prompt == "" {
		prompt = profile.Prompt
	}
	args := BuildWhisperCLIArgs(audioPath, modelPath, outJsonBase, lang, prompt, profile.Processors, profile.Threads, profile.Greedy)
	cmd := exec.CommandContext(ctx, bin, args...)
	var errBuf bytes.Buffer
	if verbose && !quiet {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		fmt.Printf("   Executing whisper-cli: %s %s\n", bin, strings.Join(args, " "))
	} else {
		cmd.Stderr = &errBuf
		cmd.Stdout = nil
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		detail := strings.TrimSpace(errBuf.String())
		if detail != "" {
			return nil, fmt.Errorf("whisper-cli execution failed: %w: %s", err, detail)
		}
		return nil, fmt.Errorf("whisper-cli execution failed: %w", err)
	}
	data, err := os.ReadFile(outJsonFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read whisper-cli output JSON (%s): %w", outJsonFile, err)
	}
	return ParseWhisperCLIJSON(data)
}
