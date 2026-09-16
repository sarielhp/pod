package tui

import (
	"fmt"
	"strings"
	"time"
)

func formatFileSize(size int64) string {
	switch {
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	case size < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
	}
}

func formatDurationShort(secs float64) string {
	m := int(secs) / 60
	s := int(secs) % 60
	if m >= 60 {
		h := m / 60
		m = m % 60
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func formatRelativeAge(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < 0 {
		return "Future"
	}

	switch {
	case diff < 24*time.Hour:
		return "Today"
	case diff < 48*time.Hour:
		return "1Day"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1Day"
		}
		return fmt.Sprintf("%dDays", days)
	case diff < 30*24*time.Hour:
		weeks := int(diff.Hours() / (24 * 7))
		if weeks == 1 {
			return "1Week"
		}
		return fmt.Sprintf("%dWeeks", weeks)
	case diff < 365*24*time.Hour:
		months := int(diff.Hours() / (24 * 30))
		if months == 1 {
			return "1Month"
		}
		return fmt.Sprintf("%dMonths", months)
	default:
		years := int(diff.Hours() / (24 * 365))
		if years == 1 {
			return "1Year"
		}
		return fmt.Sprintf("%dYears", years)
	}
}

func renderHTML(html string) string {
	if html == "" {
		return ""
	}

	var result strings.Builder
	inTag, inBold, inItalic := false, false, false
	var tagBuf strings.Builder
	var textBuf strings.Builder

	flushText := func() {
		if textBuf.Len() == 0 {
			return
		}
		text := textBuf.String()
		textBuf.Reset()
		if inBold {
			result.WriteString("\033[1m" + text + "\033[22m")
		} else if inItalic {
			result.WriteString("\033[3m" + text + "\033[23m")
		} else {
			result.WriteString(text)
		}
	}

	for i := 0; i < len(html); i++ {
		c := html[i]
		if c == '<' {
			flushText()
			inTag = true
			tagBuf.Reset()
			continue
		}
		if c == '>' && inTag {
			inTag = false
			applyHTMLTag(strings.ToLower(strings.TrimSpace(tagBuf.String())), &inBold, &inItalic, &result)
			continue
		}
		if inTag {
			tagBuf.WriteByte(c)
			continue
		}
		if c == '&' {
			const maxEntityBytes = 8
			end := i + maxEntityBytes
			if end > len(html) {
				end = len(html)
			}
			if off := strings.IndexByte(html[i:end], ';'); off >= 0 {
				entity := html[i : i+off+1]
				textBuf.WriteString(decodeHTMLEntity(entity))
				i += off
				continue
			}
		}
		textBuf.WriteByte(c)
	}
	flushText()

	return result.String()
}

func decodeHTMLEntity(entity string) string {
	switch entity {
	case "&amp;":
		return "&"
	case "&lt;":
		return "<"
	case "&gt;":
		return ">"
	case "&quot;":
		return "\""
	case "&apos;":
		return "'"
	case "&nbsp;":
		return " "
	default:
		return entity
	}
}

func applyHTMLTag(tag string, inBold, inItalic *bool, result *strings.Builder) {
	switch tag {
	case "b", "strong":
		*inBold = true
	case "/b", "/strong":
		*inBold = false
	case "i", "em":
		*inItalic = true
	case "/i", "/em":
		*inItalic = false
	case "br", "br/", "br /", "p", "/p", "/div":
		result.WriteByte('\n')
	case "li":
		result.WriteString("\n  - ")
	case "/li":
		result.WriteByte('\n')
	}
}
