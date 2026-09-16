package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"pod/pkg/config"
	"pod/pkg/util"
)

func resolveActiveWhisperProfile(cfg *Config) {
	if cfg.ActiveWhisperID <= 0 && len(cfg.WhisperProfiles) > 0 {
		cfg.ActiveWhisperID = cfg.WhisperProfiles[0].ID
	}
	for _, wp := range cfg.WhisperProfiles {
		if wp.ID == cfg.ActiveWhisperID {
			engine := wp.Engine
			if engine == "" {
				engine = config.InferWhisperEngine(wp)
			}
			cfg.WhisperEngine = engine
			cfg.WhisperModel = wp.Model
			cfg.WhisperCliBinary = wp.CliBinary
			cfg.WhisperProcessors = wp.Processors
			cfg.WhisperThreads = wp.Threads
			cfg.WhisperGreedy = wp.Greedy
			if wp.URL != "" {
				cfg.WhisperURL = wp.URL
			}
			if wp.SpeedFactor > 0 {
				cfg.WhisperSpeedFactor = wp.SpeedFactor
			}
			cfg.WhisperDockerContainer = wp.DockerContainer
			cfg.WhisperLanguage = wp.Language
			cfg.WhisperPrompt = wp.Prompt
			cfg.WhisperWakeCommand = wp.WakeCommand
			return
		}
	}
}

func printWhisperProfile(w io.Writer, wp WhisperProfile, isDefault bool) {
	engine := wp.Engine
	if engine == "" {
		engine = config.InferWhisperEngine(wp)
	}
	badge := config.WhisperEngineBadge(engine)
	defaultBadge := ""
	if isDefault {
		defaultBadge = " [DEFAULT]"
	}
	fmt.Fprintf(w, "  [%d] %s %s%s\n", wp.ID, wp.Name, badge, defaultBadge)
	fmt.Fprintf(w, "      - Engine:       %s\n", engine)
	if engine == WhisperEngineLocal {
		if wp.Model != "" {
			fmt.Fprintf(w, "      - Model:        %s\n", wp.Model)
		}
		if wp.CliBinary != "" {
			fmt.Fprintf(w, "      - CLI Binary:   %s\n", wp.CliBinary)
		}
		if wp.Processors > 0 {
			fmt.Fprintf(w, "      - Processors:   %d\n", wp.Processors)
		}
		if wp.Threads > 0 {
			fmt.Fprintf(w, "      - Threads:      %d\n", wp.Threads)
		}
		if wp.Greedy {
			fmt.Fprintln(w, "      - Greedy:       true")
		}
	} else {
		if wp.URL != "" {
			fmt.Fprintf(w, "      - URL:          %s\n", wp.URL)
		}
		if wp.DockerContainer != "" {
			fmt.Fprintf(w, "      - Container:    %s\n", wp.DockerContainer)
		}
		if wp.WakeCommand != "" {
			fmt.Fprintf(w, "      - Wake Cmd:     %s\n", wp.WakeCommand)
		}
	}
	if wp.SpeedFactor > 0 {
		fmt.Fprintf(w, "      - Speed Factor: %.1f\n", wp.SpeedFactor)
	}
	if len(wp.Languages) > 0 {
		fmt.Fprintf(w, "      - Languages:    %s\n", strings.Join(wp.Languages, ", "))
	}
	if wp.Language != "" {
		fmt.Fprintf(w, "      - Language:     %s\n", wp.Language)
	}
	if wp.Prompt != "" {
		fmt.Fprintf(w, "      - Prompt:       %s\n", wp.Prompt)
	}
	fmt.Fprintln(w)
}

func listWhispers(w io.Writer, cfg Config) {
	activeID := cfg.ActiveWhisperID
	fmt.Fprintf(w, "\n%s\n", util.RepeatStr("=", 70))
	fmt.Fprintln(w, "AVAILABLE WHISPER SERVERS:")
	fmt.Fprintf(w, "%s\n", util.RepeatStr("=", 70))

	if len(cfg.WhisperProfiles) == 0 {
		fmt.Fprintln(w, "No Whisper profiles configured in configuration file.")
		fmt.Fprintln(w, "Currently using fallback/legacy configuration:")
		fb := config.GetActiveWhisperProfile(&cfg)
		printWhisperProfile(w, fb, true)
	} else {
		for _, wp := range cfg.WhisperProfiles {
			printWhisperProfile(w, wp, wp.ID == activeID)
		}
	}
	fmt.Fprintf(w, "%s\n\n", util.RepeatStr("=", 70))
}

func parseEngineFirstSpec(parts []string, nextID int, name string, engine WhisperEngine) WhisperProfile {
	wp := WhisperProfile{ID: nextID, Name: name, Engine: engine, SpeedFactor: 7.0}
	if engine == WhisperEngineLocal {
		return parseLocalWhisperSpec(wp, parts)
	}
	return parseServerWhisperSpec(wp, parts, engine)
}

// specField is the trimmed positional field, or empty when the spec is
// shorter than that. Positional specs are written by hand and routinely stop
// early, so "absent" and "left blank" mean the same thing: keep the default.
func specField(parts []string, i int) string {
	if i >= len(parts) {
		return ""
	}
	return strings.TrimSpace(parts[i])
}

// parseLocalWhisperSpec reads name:engine:model:speed:processors:threads:greedy.
func parseLocalWhisperSpec(wp WhisperProfile, parts []string) WhisperProfile {
	wp.SpeedFactor = 70.0
	wp.Model = "tiny.en"
	wp.CliBinary = "whisper-cli"
	wp.Processors = 4
	wp.Threads = 4
	wp.Greedy = true

	if v := specField(parts, 2); v != "" {
		wp.Model = v
	}
	if sf, ok := specFloat(parts, 3); ok {
		wp.SpeedFactor = sf
	}
	if p, ok := specInt(parts, 4); ok {
		wp.Processors = p
	}
	if t, ok := specInt(parts, 5); ok {
		wp.Threads = t
	}
	if v := specField(parts, 6); v != "" {
		wp.Greedy = v != "false"
	}
	return wp
}

// parseServerWhisperSpec reads name:engine:url:speed then either
// container:language:prompt for docker, or wake:language:prompt otherwise.
func parseServerWhisperSpec(wp WhisperProfile, parts []string, engine WhisperEngine) WhisperProfile {
	wp.URL = specField(parts, 2)
	if sf, ok := specFloat(parts, 3); ok {
		wp.SpeedFactor = sf
	}
	if engine == WhisperEngineDocker {
		wp.DockerContainer = specField(parts, 4)
	} else {
		wp.WakeCommand = specField(parts, 4)
	}
	wp.Language = specField(parts, 5)
	wp.Prompt = specField(parts, 6)
	return wp
}

func specFloat(parts []string, i int) (float64, bool) {
	v := specField(parts, i)
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

func specInt(parts []string, i int) (int, bool) {
	v := specField(parts, i)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}

func parseURLFirstSpec(parts []string, nextID int, name, url string) WhisperProfile {
	sf := 7.0
	if len(parts) > 2 && strings.TrimSpace(parts[2]) != "" {
		if v, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64); err == nil {
			sf = v
		}
	}
	var container, lang, prompt, wakeCmd, model string
	var engine WhisperEngine
	if len(parts) > 3 {
		container = strings.TrimSpace(parts[3])
	}
	if len(parts) > 4 {
		lang = strings.TrimSpace(parts[4])
	}
	if len(parts) > 5 {
		prompt = strings.TrimSpace(parts[5])
	}
	if len(parts) > 6 {
		wakeCmd = strings.TrimSpace(parts[6])
	}
	if len(parts) > 7 {
		engine = WhisperEngine(strings.ToLower(strings.TrimSpace(parts[7])))
	}
	if len(parts) > 8 {
		model = strings.TrimSpace(parts[8])
	}
	wp := WhisperProfile{
		ID:              nextID,
		Name:            name,
		URL:             url,
		SpeedFactor:     sf,
		DockerContainer: container,
		Language:        lang,
		Prompt:          prompt,
		WakeCommand:     wakeCmd,
		Engine:          engine,
		Model:           model,
	}
	if wp.Engine == "" {
		wp.Engine = config.InferWhisperEngine(wp)
	}
	return wp
}

func parseWhisperProfileSpec(spec string, nextID int) WhisperProfile {
	parts := strings.Split(spec, "|")
	if len(parts) < 2 {
		fatalError("Error: Invalid Whisper server spec. Format: Name|Engine|... or Name|URL|...\n")
	}
	name := strings.TrimSpace(parts[0])
	sec := strings.TrimSpace(parts[1])
	if name == "" || sec == "" {
		fatalError("Error: Name and engine/URL cannot be empty in Whisper server spec.\n")
	}
	lowerSec := strings.ToLower(sec)
	if lowerSec == "local" || lowerSec == "docker" || lowerSec == "remote" || lowerSec == "gemini" {
		return parseEngineFirstSpec(parts, nextID, name, WhisperEngine(lowerSec))
	}
	return parseURLFirstSpec(parts, nextID, name, sec)
}

func addWhisperProfile(w io.Writer, cfg *Config, spec string) {
	nextID := 1
	for _, wp := range cfg.WhisperProfiles {
		if wp.ID >= nextID {
			nextID = wp.ID + 1
		}
	}
	newProfile := parseWhisperProfileSpec(spec, nextID)
	cfg.WhisperProfiles = append(cfg.WhisperProfiles, newProfile)
	if cfg.ActiveWhisperID <= 0 {
		cfg.ActiveWhisperID = nextID
	}
	resolveActiveWhisperProfile(cfg)
	_ = config.SaveConfig(cfg)
	badge := config.WhisperEngineBadge(newProfile.Engine)
	fmt.Fprintf(w, "Added Whisper server profile [%d] %s %s\n", nextID, newProfile.Name, badge)
}

func removeWhisperProfile(w io.Writer, cfg *Config, targetID int) {
	foundIndex := -1
	for i, wp := range cfg.WhisperProfiles {
		if wp.ID == targetID {
			foundIndex = i
			break
		}
	}
	if foundIndex == -1 {
		fatalError("Error: Whisper server profile [%d] not found in configuration.\n", targetID)
	}
	profileName := cfg.WhisperProfiles[foundIndex].Name
	cfg.WhisperProfiles = append(cfg.WhisperProfiles[:foundIndex], cfg.WhisperProfiles[foundIndex+1:]...)
	if cfg.ActiveWhisperID == targetID {
		if len(cfg.WhisperProfiles) > 0 {
			cfg.ActiveWhisperID = cfg.WhisperProfiles[0].ID
		} else {
			cfg.ActiveWhisperID = 0
		}
	}
	resolveActiveWhisperProfile(cfg)
	_ = config.SaveConfig(cfg)
	fmt.Fprintf(w, "Removed Whisper server profile [%d] %s\n", targetID, profileName)
}

func setDefaultWhisperProfile(w io.Writer, cfg *Config, targetID int) {
	if targetID == 0 {
		cfg.ActiveWhisperID = 0
		resolveActiveWhisperProfile(cfg)
		_ = config.SaveConfig(cfg)
		fmt.Fprintln(w, "Default Whisper server updated to fallback/legacy configuration.")
		return
	}
	for _, wp := range cfg.WhisperProfiles {
		if wp.ID == targetID {
			cfg.ActiveWhisperID = targetID
			resolveActiveWhisperProfile(cfg)
			_ = config.SaveConfig(cfg)
			engine := wp.Engine
			if engine == "" {
				engine = config.InferWhisperEngine(wp)
			}
			badge := config.WhisperEngineBadge(engine)
			fmt.Fprintf(w, "Default Whisper server profile updated to [%d] %s %s\n", targetID, wp.Name, badge)
			return
		}
	}
	fatalError("Error: Whisper server profile [%d] not found in configuration.\n", targetID)
}
