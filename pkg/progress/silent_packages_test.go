package progress

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// silentPackages must not write to stdout or stderr directly. They describe
// what they are doing through a Reporter and let the caller decide where it
// goes — the TUI's caller, for one, is a full-screen alt-screen that a stray
// fmt.Println corrupts. New packages in the podcast library belong on this
// list; the processing packages (pipeline, adremoval, remote) do not qualify
// yet and are tracked in review/002.md.
var silentPackages = []string{"../podcast", "../backend", "../podsite"}

// Writing to an io.Writer the caller supplied is fine, so these must match the
// unqualified builtins and the fmt helpers that target the process streams —
// never fmt.Fprintf and friends.
var bannedCalls = []string{
	"fmt.Print(",
	"fmt.Printf(",
	"fmt.Println(",
	"os.Stdout",
	"os.Stderr",
	"print(",
	"println(",
}

// standaloneCall reports whether call appears in code as its own identifier
// rather than as the tail of a longer one, so "println(" does not match inside
// "fmt.Fprintln(".
func standaloneCall(code, call string) bool {
	for i := 0; ; {
		j := strings.Index(code[i:], call)
		if j < 0 {
			return false
		}
		at := i + j
		if at == 0 || !isIdentRune(rune(code[at-1])) {
			return true
		}
		i = at + 1
	}
}

func isIdentRune(r rune) bool {
	return r == '_' || r == '.' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func TestLibraryPackagesDoNotWriteToTheTerminal(t *testing.T) {
	for _, pkgDir := range silentPackages {
		entries, err := os.ReadDir(pkgDir)
		if err != nil {
			t.Fatalf("read %s: %v", pkgDir, err)
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			checkFileIsSilent(t, filepath.Join(pkgDir, name))
		}
	}
}

// checkFileIsSilent reports every line of one file that writes to the
// terminal directly.
func checkFileIsSilent(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		code, _, _ := strings.Cut(line, "//")
		for _, banned := range bannedCalls {
			if !standaloneCall(code, banned) {
				continue
			}
			t.Errorf("%s:%d writes to the terminal (%s); report through a progress.Reporter instead:\n\t%s",
				path, i+1, strings.TrimSuffix(banned, "("), strings.TrimSpace(line))
		}
	}
}
