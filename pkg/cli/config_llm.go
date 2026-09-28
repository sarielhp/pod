package cli

import (
	"fmt"
	"io"

	"pod/pkg/config"
	"pod/pkg/util"
)

func listProfiles(w io.Writer, cfg Config) {
	activeProfile, _ := config.SelectLLMProfile(&cfg, "")
	activeID := activeProfile.ID
	fmt.Fprintf(w, "\n%s\n", util.RepeatStr("=", 70))
	fmt.Fprintln(w, "AVAILABLE LLM PROFILES & PRICING:")
	fmt.Fprintf(w, "%s\n", util.RepeatStr("=", 70))

	for _, p := range cfg.Profiles {
		isDefault := p.ID == activeID
		defaultBadge := ""
		if isDefault {
			defaultBadge = " [DEFAULT]"
		}
		hasKey := ""
		if config.ResolveLLMAPIKey(p, &cfg) != "" {
			hasKey = " (Key set)"
		}
		costInfo := config.GetProfileCost(p)

		headerStr := fmt.Sprintf("  [%d] %s", p.ID, p.Name)
		headerStr += defaultBadge
		if isDefault {
			headerStr = util.BoldGreen(headerStr)
		}
		fmt.Fprintln(w, headerStr)
		fmt.Fprintf(w, "      - Model:     %s\n", p.Model)
		fmt.Fprintf(w, "      - Type:      %s%s\n", p.Type, hasKey)
		fmt.Fprintf(w, "      - Pricing:   %s\n", costInfo.CostStr)
		fmt.Fprintf(w, "      - Est. 1-Hr: %s\n", costInfo.Est1HStr)
		fmt.Fprintf(w, "      - URL:       %s\n", p.URL)
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "%s\n\n", util.RepeatStr("=", 70))
}

func setDefaultProfile(w io.Writer, cfg *Config, targetID int) error {
	if err := config.SetDefaultProfile(cfg, targetID); err != nil {
		return fmt.Errorf("profile ID [%d] not found in configuration", targetID)
	}
	if err := saveConfig(cfg); err != nil {
		return err
	}
	for _, p := range cfg.Profiles {
		if p.ID == targetID {
			fmt.Fprintf(w, "Default LLM profile updated to [%d] %s\n", targetID, p.Name)
			return nil
		}
	}
	return nil
}
