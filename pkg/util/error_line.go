package util

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	ansiBoldRed = "\x1b[1;31m"
	ansiReset   = "\x1b[0m"
)

// FprintError writes an error so that it cannot be missed: on a line of its own,
// after a blank line, and in bold red when w is a terminal. The blank line keeps
// it apart from whatever was being drawn just before, such as a progress line, and
// from the messages that follow. The message is given without its own leading or
// trailing newlines.
func FprintError(w io.Writer, format string, args ...any) {
	msg := strings.Trim(fmt.Sprintf(format, args...), "\n")
	if msg == "" {
		return
	}
	if wantsColor(w) {
		msg = ansiBoldRed + msg + ansiReset
	}
	fmt.Fprintf(w, "\n%s\n", msg)
}

// Errorf is FprintError to standard error.
func Errorf(format string, args ...any) {
	FprintError(os.Stderr, format, args...)
}

// wantsColor reports whether colour escapes belong in what is written to w: only
// for a terminal, and not when NO_COLOR is set (https://no-color.org) or the
// terminal declares itself unable. The colour library's own switch is not used
// because it follows standard output, and a run with output piped to a file and
// errors on the screen should still show them in red.
func wantsColor(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
