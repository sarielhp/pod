package detect

import (
	"fmt"
	"net/url"
	"pod/pkg/progress"
	"pod/pkg/types"
	"pod/pkg/util"
)

func AnnounceAdDetection(profile types.LLMProfile, rep progress.Reporter) {
	if profile.URL == "" {
		return
	}
	service := profile.Type
	if service == "" {
		service = "AI service"
	}
	if u, err := url.Parse(profile.URL); err == nil && u.Hostname() != "" {
		service += " on " + u.Hostname()
	}
	progress.Or(rep).Infof("\n%s", util.BoldCyan(fmt.Sprintf("Ad detection: %s (model: %s)", service, profile.Model)))
}
