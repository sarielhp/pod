package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"pod/pkg/config"
	"pod/pkg/detect"
	"pod/pkg/episode"
	"pod/pkg/format"
	"pod/pkg/progress"
	"pod/pkg/types"
	"pod/pkg/util"
)

const (
	speakerNameMaxLen      = 60
	speakerOpeningSec      = 240
	speakerExcerptsEach    = 4
	speakerExcerptMinWords = 10
	speakerPromptMaxChars  = 24000
)

// NameSpeakersRequest gives the speakers of an existing transcript names.
type NameSpeakersRequest struct {
	// Path is a transcript JSON, or a media file whose transcript sits beside it.
	Path string

	// Set names labels directly, as label to name. When it is non-empty no model
	// is asked; the names are merged over the ones already recorded.
	Set map[string]string

	// Clear removes every recorded name.
	Clear bool

	// Profile, Model and Timeout choose the LLM as they do for ad detection.
	Profile string
	Model   string
	Timeout string

	// DryRun works the names out and reports them without writing anything.
	DryRun bool

	// Markdown also writes the readable transcript beside the JSON.
	Markdown bool
}

// NameSpeakersResult reports the names now in force.
type NameSpeakersResult struct {
	TranscriptPath string
	Names          map[string]string
	Speakers       []string
	Written        []string
}

// NameSpeakers records who the speakers of a transcript are, either as given or
// as a model works them out from what was said. The labels in the transcript are
// never rewritten: names live beside them, so a wrong guess is one --clear away.
func NameSpeakers(req NameSpeakersRequest, cfg types.Config, rep progress.Reporter) (NameSpeakersResult, error) {
	r := progress.Or(rep)
	var res NameSpeakersResult
	path, err := TranscriptPathFor(req.Path)
	if err != nil {
		return res, err
	}
	res.TranscriptPath = path
	td, err := LoadTranscriptFile(path)
	if err != nil {
		return res, err
	}
	if len(td.Speakers) == 0 {
		return res, fmt.Errorf("%s has no speaker labels; make one with `pod transcribe --speakers`", filepath.Base(path))
	}
	res.Speakers = td.Speakers

	names, err := chooseSpeakerNames(req, td, path, cfg, r)
	if err != nil {
		return res, err
	}
	res.Names = names
	if req.DryRun {
		return res, nil
	}
	td.SpeakerNames = names
	res.Written, err = writeSpeakerNames(path, td, req.Markdown)
	return res, err
}

func chooseSpeakerNames(req NameSpeakersRequest, td *types.TranscriptionData, path string, cfg types.Config, r progress.Reporter) (map[string]string, error) {
	switch {
	case req.Clear:
		return nil, nil
	case len(req.Set) > 0:
		return mergeSpeakerNames(td, req.Set)
	}
	return askModelForSpeakerNames(req, td, path, cfg, r)
}

// mergeSpeakerNames applies names given by hand over those already recorded,
// refusing a label the transcript does not have, since that is a typo and would
// otherwise vanish silently.
func mergeSpeakerNames(td *types.TranscriptionData, set map[string]string) (map[string]string, error) {
	valid := map[string]bool{}
	for _, l := range td.Speakers {
		valid[l] = true
	}
	names := map[string]string{}
	for l, n := range td.SpeakerNames {
		names[l] = n
	}
	for label, name := range set {
		if !valid[label] {
			return nil, fmt.Errorf("no speaker %q in this transcript; it has %s", label, strings.Join(td.Speakers, ", "))
		}
		if name = cleanSpeakerName(name); name == "" {
			delete(names, label)
			continue
		}
		names[label] = name
	}
	return names, nil
}

func askModelForSpeakerNames(req NameSpeakersRequest, td *types.TranscriptionData, path string, cfg types.Config, r progress.Reporter) (map[string]string, error) {
	profile, err := config.SelectLLMProfile(&cfg, req.Profile)
	if err != nil {
		return nil, err
	}
	if req.Model != "" {
		profile.Model = strings.TrimSpace(req.Model)
		profile.Name = profile.Model
	}
	timeout := detect.DefaultLLMTimeout
	if req.Timeout != "" {
		d, err := time.ParseDuration(strings.TrimSpace(req.Timeout))
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("invalid timeout %q", req.Timeout)
		}
		timeout = d
	}
	r.Infof("Naming %d speaker(s) in %s with %s...", len(td.Speakers), filepath.Base(path), profile.Name)
	reply, err := detect.CallLLMChat(profile, speakerSystemPrompt, speakerPrompt(td, speakerContext(path)), 0, timeout, config.ResolveLLMAPIKey(profile, &cfg))
	if err != nil {
		return nil, fmt.Errorf("naming speakers: %w", err)
	}
	return parseSpeakerNames(reply, td.Speakers)
}

const speakerSystemPrompt = "You work out who is speaking in a podcast transcript. You answer with a single JSON object and nothing else."

// speakerContext is what is known about the episode besides its words: the
// podcast it belongs to and its title, when the library recorded one.
func speakerContext(transcriptPath string) string {
	var parts []string
	if dir := filepath.Base(filepath.Dir(transcriptPath)); dir != "." && dir != "" {
		parts = append(parts, "Podcast: "+strings.ReplaceAll(dir, "_", " "))
	}
	audio := strings.TrimSuffix(transcriptPath, ".transcript.json") + ".mp3"
	if id := episode.LoadIdentity(audio); id != nil && id.Title != "" {
		parts = append(parts, "Episode title: "+id.Title)
	}
	return strings.Join(parts, "\n")
}

// speakerPrompt shows the model who spoke how much, the opening of the episode
// (where introductions are), and a few long utterances from each speaker.
func speakerPrompt(td *types.TranscriptionData, context string) string {
	var b strings.Builder
	if context != "" {
		b.WriteString(context + "\n\n")
	}
	b.WriteString(`The transcript below was split into speakers by software, which labels them SPEAKER_00, SPEAKER_01 and so on. Work out who each one is.

Rules:
- Give a personal name when there is evidence for it, and prefer the name to a role when there is. The evidence is: the speaker introducing themselves ("I am Dana Levy"), another speaker addressing them directly ("Thanks, Dana"), or a sign-off or credit naming them. The episode title may also name a guest.
- A name mentioned only while talking about someone ("as Mohammed said", "I spoke to Dana") is not evidence that the speaker is that person. Never guess a name from the topic or the voice.
- Otherwise give a role: "Host", "Guest", "Reporter", "Narrator", "Ad voice" or "Promo".
- The software sometimes splits one person into two labels. Give both the same name.
- If a speaker says too little to tell, use null.
- Keep names and roles short, in the transcript's own script for names.

Reply with a JSON object mapping every label to a string or null, for example {"SPEAKER_00": "Dana Levy", "SPEAKER_01": "Host", "SPEAKER_02": null}.

`)
	b.WriteString(speakerTalkTable(td))
	b.WriteString("\nOpening of the episode:\n")
	b.WriteString(speakerOpening(td))
	b.WriteString("\nSample utterances from each speaker:\n")
	b.WriteString(speakerExcerpts(td))
	out := b.String()
	if len(out) > speakerPromptMaxChars {
		out = out[:speakerPromptMaxChars]
	}
	return out
}

func speakerTalkTable(td *types.TranscriptionData) string {
	talk := map[string]float64{}
	var total float64
	for _, s := range td.Segments {
		talk[s.Speaker] += s.End - s.Start
		total += s.End - s.Start
	}
	var b strings.Builder
	b.WriteString("Speakers and how long each talks:\n")
	for _, l := range td.Speakers {
		fmt.Fprintf(&b, "%s: %s (%.0f%%)\n", l, format.FormatClock(talk[l]), 100*talk[l]/maxFloat(total, 1))
	}
	return b.String()
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func speakerOpening(td *types.TranscriptionData) string {
	var b strings.Builder
	for _, s := range td.Segments {
		if s.Start > speakerOpeningSec {
			break
		}
		fmt.Fprintf(&b, "[%s] %s\n", s.Speaker, strings.TrimSpace(s.Text))
	}
	return b.String()
}

func speakerExcerpts(td *types.TranscriptionData) string {
	by := map[string][]types.TranscriptionSegment{}
	for _, s := range td.Segments {
		if len(strings.Fields(s.Text)) >= speakerExcerptMinWords {
			by[s.Speaker] = append(by[s.Speaker], s)
		}
	}
	var b strings.Builder
	for _, l := range td.Speakers {
		segs := by[l]
		for i := 0; i < speakerExcerptsEach && i < len(segs); i++ {
			s := segs[i*len(segs)/speakerExcerptsEach]
			fmt.Fprintf(&b, "[%s] (%s) %s\n", l, format.FormatClock(s.Start), strings.TrimSpace(s.Text))
		}
	}
	return b.String()
}

var jsonObject = regexp.MustCompile(`(?s)\{.*\}`)

// parseSpeakerNames reads the model's reply. Anything that is not one of the
// transcript's labels, or has no usable name, is dropped rather than trusted.
func parseSpeakerNames(reply string, labels []string) (map[string]string, error) {
	raw := jsonObject.FindString(reply)
	if raw == "" {
		return nil, fmt.Errorf("the model did not answer with a JSON object: %.120q", reply)
	}
	var got map[string]*string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return nil, fmt.Errorf("could not read the model's answer: %w", err)
	}
	names := map[string]string{}
	for _, l := range labels {
		if p := got[l]; p != nil {
			if name := cleanSpeakerName(*p); name != "" && !strings.EqualFold(name, "null") {
				names[l] = name
			}
		}
	}
	return names, nil
}

func cleanSpeakerName(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > speakerNameMaxLen {
		s = string(r[:speakerNameMaxLen])
	}
	return s
}

// writeSpeakerNames records the names in the transcript JSON, changing nothing
// else in it (the file also carries fields this program does not model), and
// refreshes the readable transcript when asked.
func writeSpeakerNames(path string, td *types.TranscriptionData, markdown bool) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc := map[string]any{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if len(td.SpeakerNames) == 0 {
		delete(doc, "speaker_names")
	} else {
		doc["speaker_names"] = td.SpeakerNames
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := util.WriteFileAtomic(path, append(out, '\n'), 0o644); err != nil {
		return nil, err
	}
	written := []string{path}
	if !markdown {
		return written, nil
	}
	mdPath := strings.TrimSuffix(path, ".transcript.json") + ".transcript.md"
	title := filepath.Base(strings.TrimSuffix(path, ".transcript.json"))
	md, err := format.ConvertToReadable(td, title, TranscriptDuration(td), mdPath, true)
	if err != nil {
		return written, err
	}
	return append(written, md), nil
}
