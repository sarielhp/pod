package util

import (
	"net"
	"strings"
)

var netInterfaceAddrs = net.InterfaceAddrs

func ExtractHost(url string) string {
	protoEnd := -1
	for i := 0; i < len(url)-2; i++ {
		if url[i:i+3] == "://" {
			protoEnd = i + 3
			break
		}
	}
	if protoEnd < 0 {
		protoEnd = 0
	}
	hostEnd := protoEnd
	for hostEnd < len(url) && url[hostEnd] != '/' && url[hostEnd] != ':' {
		hostEnd++
	}
	return url[protoEnd:hostEnd]
}

func IsLocalHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "0.0.0.0", "::1":
		return true
	}
	addrs, err := netInterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.String() == host {
				return true
			}
		}
	}
	return false
}

func SplitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func SplitTab(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\t' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	if start <= len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

func ToLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}

func ZeroWipeKey(key string) string {
	if len(key) == 0 {
		return ""
	}
	b := make([]byte, len(key))
	for i := range b {
		b[i] = '0'
	}
	return string(b)
}

func IsZeroedKey(key string) bool {
	if key == "" {
		return true
	}
	for i := 0; i < len(key); i++ {
		if key[i] != '0' {
			return false
		}
	}
	return true
}

func RepeatStr(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}

func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	var sb strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			inEsc = true
			i++
			continue
		}
		if inEsc {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				inEsc = false
			}
			continue
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

func isWideRune(r rune) bool {
	if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2300 && r <= 0x23FF) {
		return true
	}
	if r == 0x2728 || r == 0x2702 || r == 0x2B07 || r == 0x26A1 {
		return true
	}
	return r >= 0x4E00 && r <= 0x9FFF
}

func runeDisplayWidth(r rune) int {
	if r == 0xFE0F || r == 0xFE0E || (r >= 0x200B && r <= 0x200D) || (r >= 0x0300 && r <= 0x036F) || (r >= 0x0591 && r <= 0x05C7) {
		return 0
	}
	if isWideRune(r) {
		return 2
	}
	return 1
}

func StringDisplayWidth(s string) int {
	clean := stripANSI(s)
	w := 0
	for _, r := range clean {
		w += runeDisplayWidth(r)
	}
	return w
}

func PadRight(s string, width int) string {
	w := StringDisplayWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func StripExt(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[:i]
		}
		if path[i] == '/' || path[i] == '\\' {
			break
		}
	}
	return path
}

func FilepathBase(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}
