package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"pod/pkg/config"
	"pod/pkg/util"
)

func migratePodcastsManagerConfig(w io.Writer, cfg *Config) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	pmConfigDir := filepath.Join(home, ".config", "podcasts_manager")
	pmConfigPath := filepath.Join(pmConfigDir, "config.json")
	if _, statErr := os.Stat(pmConfigPath); os.IsNotExist(statErr) {
		pmConfigDir = filepath.Join(home, ".config", "podcast_manager")
		pmConfigPath = filepath.Join(pmConfigDir, "config.json")
	}

	if _, statErr := os.Stat(pmConfigPath); os.IsNotExist(statErr) {
		return false
	}

	data, readErr := os.ReadFile(pmConfigPath)
	if readErr != nil {
		return false
	}

	var pmCfg struct {
		Host           string   `json:"host"`
		Token          string   `json:"token"`
		SQLiteDBPath   string   `json:"sqlite_db_path"`
		PodcastsDir    string   `json:"podcasts_dir"`
		PostProcessors []string `json:"post_processors"`
	}

	if err := json.Unmarshal(data, &pmCfg); err != nil {
		return false
	}

	modified := false
	if cfg.PodcastsDir == "" && pmCfg.PodcastsDir != "" {
		cfg.PodcastsDir = pmCfg.PodcastsDir
		modified = true
	}
	if len(cfg.PostProcessors) == 0 && len(pmCfg.PostProcessors) > 0 {
		if valid := resolvedPostProcessors(w, pmCfg.PostProcessors); len(valid) > 0 {
			cfg.PostProcessors = valid
			modified = true
		}
	}

	if modified {
		fmt.Fprintf(w, "Migrated settings from podcast_manager config '%s'\n", pmConfigPath)
	}
	return modified
}

func resolvedPostProcessors(w io.Writer, progs []string) []string {
	var out []string
	for _, prog := range progs {
		fullPath, err := resolveProcessorPath(prog)
		if err != nil {
			fmt.Fprintf(w, "Warning: skipping post-processor '%s': %v\n", prog, err)
			continue
		}
		out = append(out, fullPath)
	}
	return out
}

func handleConfigMigrate(w io.Writer, cfg *Config, source string) error {
	migrated := false
	source = strings.ToLower(strings.TrimSpace(source))

	checkPM := source == "" || source == "all" || source == "pm" || source == "podcasts_manager" || source == "podcast_manager"

	if checkPM {
		if migratePodcastsManagerConfig(w, cfg) {
			migrated = true
		}
	}

	if !migrated {
		fmt.Fprintln(w, "No legacy configuration found to migrate or settings already up-to-date.")
		return nil
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	fmt.Fprintf(w, "Configuration saved to '%s'\n", config.ConfigPath())
	return nil
}

func resolveProcessorPath(prog string) (string, error) {
	path, err := exec.LookPath(prog)
	if err == nil {
		return filepath.Abs(path)
	}
	if util.FileExists(prog) {
		return filepath.Abs(prog)
	}
	return "", fmt.Errorf("program '%s' not found or not executable", prog)
}

func handleConfigProcessor(w io.Writer, cfg *Config, cmd string, value string) error {
	switch cmd {
	case "set":
		if value == "" {
			fatalError("%s\n", "Error: missing program for 'config processor set <program>'")
		}
		fullPath, err := resolveProcessorPath(value)
		if err != nil {
			fatalError("%s\n", fmt.Sprintf("Error: failed to resolve post-processor program: %v", err))
		}
		exists := false
		for _, p := range cfg.PostProcessors {
			if p == fullPath {
				exists = true
				break
			}
		}
		if !exists {
			cfg.PostProcessors = append(cfg.PostProcessors, fullPath)
			if err := saveConfig(cfg); err != nil {
				return err
			}
		}
		fmt.Fprintf(w, "Added post-processor: %s\n", fullPath)

	case "list":
		if len(cfg.PostProcessors) == 0 {
			fmt.Fprintln(w, "No post-processors configured.")
		} else {
			fmt.Fprintln(w, "=== Configured Post-Processors ===")
			for i, p := range cfg.PostProcessors {
				fmt.Fprintf(w, "  %d. %s\n", i+1, p)
			}
		}

	case "del":
		if value == "" {
			fatalError("%s\n", "Error: missing number for 'config processor del <number>'")
		}
		idx, err := strconv.Atoi(value)
		if err != nil || idx < 1 || idx > len(cfg.PostProcessors) {
			fatalError("%s\n", fmt.Sprintf("Error: invalid post-processor number '%s'. Must be between 1 and %d.", value, len(cfg.PostProcessors)))
		}
		removed := cfg.PostProcessors[idx-1]
		cfg.PostProcessors = append(cfg.PostProcessors[:idx-1], cfg.PostProcessors[idx:]...)
		if err := saveConfig(cfg); err != nil {
			return err
		}
		fmt.Fprintf(w, "Deleted post-processor #%d: %s\n", idx, removed)

	default:
		return fmt.Errorf("unknown processor command '%s'", cmd)
	}
	return nil
}
