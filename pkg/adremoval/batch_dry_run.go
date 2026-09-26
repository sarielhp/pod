package adremoval

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"pod/pkg/pipeline"
	"pod/pkg/progress"
	"pod/pkg/types"
	"pod/pkg/util"
)

type dryRunFileStatus struct {
	path   string
	status string
}

func auditFileStatus(inputFile string, opts types.ProcOptions) (category string, statusText string) {
	mainMP3File, precutFile, _ := pipeline.ResolveAudioFiles(inputFile, opts.Verbose)
	statFile := pipeline.StatusPathFor(mainMP3File)
	st, _ := pipeline.LoadEpisodeStatus(statFile)

	if st != nil {
		switch st.Status {
		case types.StateDone, types.StateCopiedBack, types.StateArchived:
			return "completed", "Completed (Ad-Free)"
		case types.StateReadyForCopyBack:
			return "remote_pending", "Ready for Pull (Remote Done)"
		case types.StateQueuedRemote, types.StateTranscribingRemotely, types.StateCuttingRemotely, types.StateAwaitingTranscription:
			return "remote_pending", fmt.Sprintf("Remote Processing (%s)", st.Status)
		}
	}

	baseName := util.StripExt(mainMP3File)
	jsonFile := opts.TranscriptPath
	if jsonFile == "" {
		jsonFile = baseName + ".transcript.json"
	}
	cutsFile := baseName + ".cuts.json"

	if !util.FileExists(jsonFile) {
		return "needs_tx", "Needs Transcription"
	}
	if !util.FileExists(cutsFile) {
		return "needs_llm", "Needs Ad Detection (LLM)"
	}
	if util.FileExists(precutFile) {
		return "completed", "Completed (Ad-Free)"
	}
	data, err := os.ReadFile(cutsFile)
	var cd types.CutsData
	if err == nil && json.Unmarshal(data, &cd) == nil && len(cd.CutIntervals) > 0 {
		return "needs_cut", "Needs Audio Cutting"
	}
	return "completed", "Completed (0 ads)"
}

func printDryRunSummary(filesCount, needsTx, needsLLM, needsCut, remotePending, alreadyComplete int, opts types.ProcOptions, details []dryRunFileStatus, r progress.Reporter) {
	totalNeedingAction := needsTx + needsLLM + needsCut
	r.Infof("")
	r.Infof("%s", util.Bold("DRY RUN: Audio Processing Pipeline Status"))
	r.Infof("%s", strings.Repeat("─", 55))
	r.Infof("  • Total Episodes Scanned:        %d", filesCount)
	r.Infof("  • Needs Transcription (Whisper): %d", needsTx)
	r.Infof("  • Needs Ad Detection (LLM):      %d", needsLLM)
	r.Infof("  • Needs Audio Cutting (FFmpeg):  %d", needsCut)
	r.Infof("  • Already Processed / Ad-Free:   %d", alreadyComplete)
	if remotePending > 0 {
		r.Infof("  • Stranded in a remote state:    %d", remotePending)
	}
	r.Infof("%s", strings.Repeat("─", 55))
	r.Infof("  Total Needing Local Processing:  %s", util.Bold(strconv.Itoa(totalNeedingAction)))
	if opts.Count > 0 && totalNeedingAction > opts.Count {
		r.Infof("  (Limit -n %d: would process first %d of %d episodes)", opts.Count, opts.Count, totalNeedingAction)
	}
	r.Infof("")

	if opts.Verbose {
		r.Infof("Episode Details:")
		for _, d := range details {
			r.Infof("  [%s] %s", d.status, util.DisplayName(d.path))
		}
		r.Infof("")
	}
}

func handleProcDryRun(files []string, opts types.ProcOptions, cfg types.Config, r progress.Reporter) {
	var needsTranscribe, needsLLM, needsCut, alreadyComplete, remotePending int
	var details []dryRunFileStatus

	for _, inputFile := range files {
		if strings.HasSuffix(inputFile, ".json") {
			continue
		}
		cat, desc := auditFileStatus(inputFile, opts)
		details = append(details, dryRunFileStatus{path: inputFile, status: desc})
		switch cat {
		case "completed":
			alreadyComplete++
		case "remote_pending":
			remotePending++
		case "needs_tx":
			needsTranscribe++
		case "needs_llm":
			needsLLM++
		case "needs_cut":
			needsCut++
		}
	}

	printDryRunSummary(len(files), needsTranscribe, needsLLM, needsCut, remotePending, alreadyComplete, opts, details, r)
}
