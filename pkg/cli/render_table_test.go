package cli

import (
	"strings"
	"testing"
	"time"
)

func TestStringDisplayWidth(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  int
	}{
		{"1bcef", 5},
		{"\x1b[1;36m1bcef\x1b[0m", 5},
		{"5-4", 3},
		{"🎙️", 2},
		{"✨", 2},
		{"⬇️", 2},
		{"✂️", 2},
		{"⏳", 2},
		{"📅", 2},
		{"Top 3", 5},
		{"2026-09-08", 10},
		{"שלום", 4},
	}

	for _, tc := range cases {
		got := stringDisplayWidth(tc.input)
		if got != tc.want {
			t.Errorf("stringDisplayWidth(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestPadCell(t *testing.T) {
	t.Parallel()
	left := padCell("abc", 5, alignLeft)
	if left != "abc  " {
		t.Errorf("alignLeft want %q, got %q", "abc  ", left)
	}

	right := padCell("abc", 5, alignRight)
	if right != "  abc" {
		t.Errorf("alignRight want %q, got %q", "  abc", right)
	}

	center := padCell("ab", 6, alignCenter)
	if center != "  ab  " {
		t.Errorf("alignCenter want %q, got %q", "  ab  ", center)
	}

	emojiCenter := padCell("✨", 4, alignCenter)
	if stringDisplayWidth(emojiCenter) != 4 {
		t.Errorf("emojiCenter width want 4, got %d (%q)", stringDisplayWidth(emojiCenter), emojiCenter)
	}
}

func TestRenderTableBordersConnected(t *testing.T) {
	t.Parallel()
	cols := []tableColumn{
		{Header: "ID", Width: 5, Align: alignCenter},
		{Header: "Title", Width: 10, Align: alignLeft},
		{Header: "✨", Width: 4, Align: alignCenter},
	}

	top := renderTableTop(cols)
	hdr := renderTableHeader(cols)
	div := renderTableDivider(cols)
	row := renderTableRow([]string{"12345", "Test Title", "0"}, cols)
	bot := renderTableBottom(cols)

	expectedWidth := stringDisplayWidth(top)
	if stringDisplayWidth(hdr) != expectedWidth {
		t.Errorf("hdr width %d != top width %d", stringDisplayWidth(hdr), expectedWidth)
	}
	if stringDisplayWidth(div) != expectedWidth {
		t.Errorf("div width %d != top width %d", stringDisplayWidth(div), expectedWidth)
	}
	if stringDisplayWidth(row) != expectedWidth {
		t.Errorf("row width %d != top width %d", stringDisplayWidth(row), expectedWidth)
	}
	if stringDisplayWidth(bot) != expectedWidth {
		t.Errorf("bot width %d != top width %d", stringDisplayWidth(bot), expectedWidth)
	}

	if !strings.HasPrefix(top, "┌") || !strings.HasSuffix(top, "┐") {
		t.Errorf("invalid top border: %s", top)
	}
	if !strings.HasPrefix(div, "├") || !strings.HasSuffix(div, "┤") {
		t.Errorf("invalid divider border: %s", div)
	}
	if !strings.HasPrefix(bot, "└") || !strings.HasSuffix(bot, "┘") {
		t.Errorf("invalid bottom border: %s", bot)
	}
	if !strings.HasPrefix(row, "│") || !strings.HasSuffix(row, "│") {
		t.Errorf("invalid row border: %s", row)
	}
}

func TestCompactDownloadPolicy(t *testing.T) {
	t.Parallel()
	if compactDownloadPolicy("latest", 0) != "New" {
		t.Errorf("expected 'New'")
	}
	if compactDownloadPolicy("all", 0) != "All" {
		t.Errorf("expected 'All'")
	}
	if compactDownloadPolicy("none", 0) != "Off" {
		t.Errorf("expected 'Off'")
	}
	if compactDownloadPolicy("latest_k", 5) != "Top 5" {
		t.Errorf("expected 'Top 5'")
	}
}

func TestCompactAdRemoval(t *testing.T) {
	t.Parallel()
	if compactAdRemoval("all") != "All" {
		t.Errorf("expected 'All'")
	}
	if compactAdRemoval("none") != "Off" {
		t.Errorf("expected 'Off'")
	}
	if compactAdRemoval("latest") != "New" {
		t.Errorf("expected 'New'")
	}
}

func TestFormatRelativeDate(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	now := time.Date(2026, 9, 8, 14, 30, 0, 0, loc)

	cases := []struct {
		input time.Time
		want  string
	}{
		{time.Time{}, "-"},
		{time.Date(2026, 9, 8, 9, 0, 0, 0, loc), "today"},
		{time.Date(2026, 9, 7, 23, 59, 0, 0, loc), "yesterday"},
		{time.Date(2026, 9, 6, 10, 0, 0, 0, loc), "last week (-2 d)"},
		{time.Date(2026, 9, 5, 12, 0, 0, 0, loc), "last week (-3 d)"},
		{time.Date(2026, 9, 4, 8, 0, 0, 0, loc), "last week (-4 d)"},
		{time.Date(2026, 9, 1, 10, 0, 0, 0, loc), "last week (-7 d)"},
		{time.Date(2026, 8, 31, 10, 0, 0, 0, loc), "2026-08-31"},
		{time.Date(2026, 9, 9, 10, 0, 0, 0, loc), "2026-09-09"},
	}

	for _, tc := range cases {
		got := formatRelativeDateAt(tc.input, now)
		if got != tc.want {
			t.Errorf("formatRelativeDateAt(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatRelativeDateStr(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	now := time.Date(2026, 9, 8, 14, 30, 0, 0, loc)

	cases := []struct {
		input string
		want  string
	}{
		{"", "-"},
		{"-", "-"},
		{"invalid", "invalid"},
		{"2026-09-08", "today"},
		{"2026-09-07", "yesterday"},
		{"2026-09-04", "last week (-4 d)"},
		{"2026-08-31", "2026-08-31"},
		{"2026-09-07T12:00:00Z", "yesterday"},
	}

	for _, tc := range cases {
		got := formatRelativeDateStrAt(tc.input, now)
		if got != tc.want {
			t.Errorf("formatRelativeDateStrAt(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatRelativeDateTime(t *testing.T) {
	t.Parallel()
	loc := time.UTC
	now := time.Date(2026, 9, 8, 14, 30, 0, 0, loc)

	cases := []struct {
		input time.Time
		want  string
	}{
		{time.Time{}, "-"},
		{time.Date(2026, 9, 8, 9, 15, 0, 0, loc), "today 09:15"},
		{time.Date(2026, 9, 7, 20, 0, 0, 0, loc), "yesterday 20:00"},
		{time.Date(2026, 9, 4, 8, 0, 0, 0, loc), "last week (-4 d)"},
		{time.Date(2026, 8, 31, 10, 5, 0, 0, loc), "2026-08-31 10:05"},
	}

	for _, tc := range cases {
		got := formatRelativeDateTimeAt(tc.input, now)
		if got != tc.want {
			t.Errorf("formatRelativeDateTimeAt(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
