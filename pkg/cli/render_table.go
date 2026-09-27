package cli

import (
	"fmt"
	"pod/pkg/format"
	"pod/pkg/util"
	"strconv"
	"strings"
	"time"
)

type tableAlign int

const (
	alignLeft tableAlign = iota
	alignCenter
	alignRight
)

type tableColumn struct {
	Header string
	Width  int
	Align  tableAlign
}

func stringDisplayWidth(s string) int {
	return util.StringDisplayWidth(s)
}

func padCell(val string, width int, align tableAlign) string {
	w := util.StringDisplayWidth(val)
	if w >= width {
		return val
	}
	diff := width - w
	switch align {
	case alignCenter:
		left := diff / 2
		right := diff - left
		return strings.Repeat(" ", left) + val + strings.Repeat(" ", right)
	case alignRight:
		return strings.Repeat(" ", diff) + val
	default:
		return val + strings.Repeat(" ", diff)
	}
}

func renderTableTop(cols []tableColumn) string {
	var parts []string
	for _, c := range cols {
		parts = append(parts, strings.Repeat("─", c.Width+2))
	}
	return "┌" + strings.Join(parts, "┬") + "┐"
}

func renderTableHeader(cols []tableColumn) string {
	var formatted []string
	for _, c := range cols {
		formatted = append(formatted, padCell(c.Header, c.Width, alignCenter))
	}
	return "│ " + strings.Join(formatted, " │ ") + " │"
}

func renderTableDivider(cols []tableColumn) string {
	var parts []string
	for _, c := range cols {
		parts = append(parts, strings.Repeat("─", c.Width+2))
	}
	return "├" + strings.Join(parts, "┼") + "┤"
}

func renderTableRow(cells []string, cols []tableColumn) string {
	var formatted []string
	for i, c := range cols {
		val := ""
		if i < len(cells) {
			val = cells[i]
		}
		formatted = append(formatted, padCell(val, c.Width, c.Align))
	}
	return "│ " + strings.Join(formatted, " │ ") + " │"
}

func renderTableBottom(cols []tableColumn) string {
	var parts []string
	for _, c := range cols {
		parts = append(parts, strings.Repeat("─", c.Width+2))
	}
	return "└" + strings.Join(parts, "┴") + "┘"
}

func compactDownloadPolicy(policy string, k int) string {
	switch strings.ToLower(policy) {
	case DownloadPolicyLatest:
		return "New"
	case DownloadPolicyAll:
		return "All"
	case DownloadPolicyNone:
		return "Off"
	case DownloadPolicyLatestK:
		if k <= 0 {
			k = 3
		}
		return fmt.Sprintf("Top %d", k)
	default:
		if policy == "" {
			return "New"
		}
		return policy
	}
}

func compactAdRemoval(adRemoval string) string {
	switch strings.ToLower(adRemoval) {
	case AdRemovalAll:
		return "All"
	case AdRemovalNone:
		return "Off"
	case AdRemovalLatest:
		return "New"
	default:
		if adRemoval == "" {
			return "All"
		}
		return adRemoval
	}
}

func podcastTableColumns(titleWidth int) []tableColumn {
	return []tableColumn{
		{Header: "ID", Width: 5, Align: alignCenter},
		{Header: "Title", Width: titleWidth, Align: alignLeft},
		{Header: "🎙️", Width: 4, Align: alignRight},
		{Header: "✨", Width: 4, Align: alignRight},
		{Header: "⬇️", Width: 5, Align: alignCenter},
		{Header: "✂️", Width: 5, Align: alignCenter},
		{Header: "⏳", Width: 4, Align: alignCenter},
		{Header: "📅 Last", Width: 16, Align: alignCenter},
	}
}

func latestEpisodeTableColumns(podWidth, titleWidth int) []tableColumn {
	return []tableColumn{
		{Header: "📅 Date", Width: 16, Align: alignCenter},
		{Header: "🎙️ Pod", Width: 6, Align: alignCenter},
		{Header: "🔖 Ep", Width: 6, Align: alignCenter},
		{Header: "📻 Podcast", Width: podWidth, Align: alignLeft},
		{Header: "✂️ AdR", Width: 9, Align: alignCenter},
		{Header: "⏱️ Dur", Width: 6, Align: alignRight},
		{Header: "Title", Width: titleWidth, Align: alignLeft},
	}
}

func buildPodcastRowCells(item lsPodcastItem, titleWidth int) []string {
	pName := util.TruncateDisplayName(item.Title, titleWidth)
	dlStr := compactDownloadPolicy(item.DownloadPolicy, 3)
	adStr := compactAdRemoval(item.AdRemoval)
	lastDateStr := formatRelativeDateStr(item.LastEpisode)
	return []string{
		util.BoldCyan(item.ShortID),
		pName,
		strconv.Itoa(item.EpisodeCount),
		strconv.Itoa(item.CleanCount),
		dlStr,
		adStr,
		item.Retention,
		lastDateStr,
	}
}

func buildLatestEpisodeRowCells(item lsEpisodeItem, podWidth, titleWidth int) []string {
	dStr := formatRelativeDateTime(item.modTime)
	pName := util.TruncateDisplayName(item.podcastTitle, podWidth)
	shortStatus := formatShortStatus(item.statusStr)
	coloredStatus := shortStatus
	if item.statusColor == "green" {
		coloredStatus = util.BoldGreen(shortStatus)
	} else if item.statusColor == "yellow" {
		coloredStatus = util.BoldYellow(shortStatus)
	} else if item.statusColor == "cyan" {
		coloredStatus = util.Bold(shortStatus)
	}
	durStr := "-"
	if item.origDuration > 0 {
		durStr = format.FormatClock(item.origDuration)
	}
	epName := util.TruncateDisplayName(item.episodeName, titleWidth)
	return []string{
		dStr,
		item.podcastShortID,
		util.BoldCyan(item.episodeShortID),
		pName,
		coloredStatus,
		durStr,
		epName,
	}
}

func formatRelativeDateAt(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "-"
	}

	loc := now.Location()
	tLoc := t.In(loc)

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	targetDay := time.Date(tLoc.Year(), tLoc.Month(), tLoc.Day(), 0, 0, 0, 0, loc)

	if targetDay.After(today) {
		return tLoc.Format("2006-01-02")
	}

	diffHours := today.Sub(targetDay).Hours()
	days := int(diffHours+12) / 24

	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days >= 2 && days <= 7:
		return fmt.Sprintf("last week (-%d d)", days)
	default:
		return tLoc.Format("2006-01-02")
	}
}

func formatRelativeDateStrAt(dateStr string, now time.Time) string {
	if dateStr == "" || dateStr == "-" {
		return "-"
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04", time.RFC3339} {
		if pt, err := time.ParseInLocation(layout, dateStr, now.Location()); err == nil {
			return formatRelativeDateAt(pt, now)
		}
	}
	return dateStr
}

func formatRelativeDateStr(dateStr string) string {
	return formatRelativeDateStrAt(dateStr, time.Now())
}

func formatRelativeDateTimeAt(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "-"
	}

	loc := now.Location()
	tLoc := t.In(loc)

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	targetDay := time.Date(tLoc.Year(), tLoc.Month(), tLoc.Day(), 0, 0, 0, 0, loc)

	if targetDay.After(today) {
		return tLoc.Format("2006-01-02 15:04")
	}

	diffHours := today.Sub(targetDay).Hours()
	days := int(diffHours+12) / 24

	switch {
	case days == 0:
		return fmt.Sprintf("today %s", tLoc.Format("15:04"))
	case days == 1:
		return fmt.Sprintf("yesterday %s", tLoc.Format("15:04"))
	case days >= 2 && days <= 7:
		return fmt.Sprintf("last week (-%d d)", days)
	default:
		return tLoc.Format("2006-01-02 15:04")
	}
}

func formatRelativeDateTime(t time.Time) string {
	return formatRelativeDateTimeAt(t, time.Now())
}

func queueTableColumns(titleWidth int) []tableColumn {
	return []tableColumn{
		{Header: "Podcast ID", Width: 10, Align: alignCenter},
		{Header: "Episode ID", Width: 10, Align: alignCenter},
		{Header: "Pri", Width: 3, Align: alignRight},
		{Header: "Length", Width: 8, Align: alignRight},
		{Header: "P-date", Width: 19, Align: alignLeft},
		{Header: "Title", Width: titleWidth, Align: alignLeft},
	}
}
