package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiFaint   = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiBlue    = "\x1b[34m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
	ansiGray    = "\x1b[90m"
)

var colorOnStderr = shouldUseColor(os.Stderr)

func shouldUseColor(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("CLICOLOR") == "0" {
		return false
	}
	term := strings.ToLower(strings.TrimSpace(os.Getenv("TERM")))
	if term == "" || term == "dumb" {
		return false
	}
	return isTerminal(f)
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}

func paint(enabled bool, s string, codes ...string) string {
	if !enabled || s == "" || len(codes) == 0 {
		return s
	}
	var b strings.Builder
	for _, c := range codes {
		b.WriteString(c)
	}
	b.WriteString(s)
	b.WriteString(ansiReset)
	return b.String()
}

// trueColor generates an ANSI true color escape sequence for RGB values
func trueColor(r, g, b int) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

func bannerFor(f *os.File) string {
	if os.Getenv("POKE_NO_BANNER") != "" {
		return ""
	}

	plain := []string{
		" ██▓███   ▒█████   ██ ▄█▀▓█████ ",
		"▓██░  ██▒▒██▒  ██▒ ██▄█▒ ▓█   ▀ ",
		"▓██░ ██▓▒▒██░  ██▒▓███▄░ ▒███   ",
		"▒██▄█▓▒ ▒▒██   ██░▓██ █▄ ▒▓█  ▄ ",
		"▒██▒ ░  ░░ ████▓▒░▒██▒ █▄░▒████▒",
		"▒▓▒░ ░  ░░ ▒░▒░▒░ ▒ ▒▒ ▓▒░░ ▒░ ░",
		"░▒ ░       ░ ▒ ▒░ ░ ░▒ ▒░ ░ ░  ░",
		"░░       ░ ░ ░ ▒  ░ ░░ ░    ░   ",
		"             ░ ░  ░  ░      ░  ░",
	}

	useColor := shouldUseColor(f)
	if !useColor {
		return strings.Join(plain, "\n") + "\n"
	}

	// Custom palette: UI/UX-1 through UI/UX-5
	// UI/UX-1: #D9C0A3 (216, 192, 162) - light beige
	// UI/UX-2: #D9A273 (216, 162, 114) - light orange
	// UI/UX-3: #D9763D (216, 117, 60) - orange
	// UI/UX-4: #BF3E21 (191, 61, 32) - dark red-orange
	// UI/UX-5: #400702 (63, 7, 1) - very dark red
	palette := []string{
		trueColor(216, 192, 162), // UI/UX-1
		trueColor(216, 162, 114), // UI/UX-2
		trueColor(216, 117, 60),  // UI/UX-3
		trueColor(191, 61, 32),   // UI/UX-4
		trueColor(63, 7, 1),      // UI/UX-5
	}

	// Create gradient pattern across 9 lines
	// Distribute colors: 1, 1, 2, 2, 3, 3, 4, 4, 5
	colorIndices := []int{0, 0, 1, 1, 2, 2, 3, 3, 4}

	out := make([]string, 0, len(plain))
	for i, line := range plain {
		colorCode := palette[colorIndices[i]]
		out = append(out, paint(true, line, ansiBold, colorCode))
	}
	return strings.Join(out, "\n") + "\n"
}

func styledKey(name string, codes ...string) string {
	return paint(colorOnStderr, name, codes...)
}

func styledValue(s string, codes ...string) string {
	return paint(colorOnStderr, s, codes...)
}

func styledStatusCode(code int) string {
	if code == 0 {
		return styledValue("0", ansiGray)
	}

	switch {
	case code >= 200 && code <= 299:
		return styledValue(intToString(code), ansiGreen, ansiBold)
	case code >= 300 && code <= 399:
		return styledValue(intToString(code), ansiCyan, ansiBold)
	case code >= 400 && code <= 499:
		return styledValue(intToString(code), ansiYellow, ansiBold)
	case code >= 500 && code <= 599:
		return styledValue(intToString(code), ansiRed, ansiBold)
	default:
		return styledValue(intToString(code), ansiMagenta, ansiBold)
	}
}

func styledStatusKey(code int) string {
	key := "status_" + intToString(code)
	switch {
	case code >= 200 && code <= 299:
		return styledKey(key, ansiGreen, ansiBold)
	case code >= 300 && code <= 399:
		return styledKey(key, ansiCyan, ansiBold)
	case code >= 400 && code <= 499:
		return styledKey(key, ansiYellow, ansiBold)
	case code >= 500 && code <= 599:
		return styledKey(key, ansiRed, ansiBold)
	default:
		return styledKey(key, ansiMagenta, ansiBold)
	}
}

func styledCategoryKey(c MarkerCategory) string {
	key := "category_" + c.String() + "_responses"
	switch c {
	case CategoryJailbreakSuccess:
		return styledKey(key, ansiMagenta, ansiBold)
	case CategorySystemLeak:
		return styledKey(key, ansiYellow, ansiBold)
	case CategoryHTTPError:
		return styledKey(key, ansiRed, ansiBold)
	case CategoryRateLimit:
		return styledKey(key, ansiYellow, ansiBold)
	default:
		return styledKey(key, ansiCyan, ansiBold)
	}
}

func styledMarkerKey(id string) string {
	return styledKey("marker_"+id, ansiCyan, ansiBold)
}

func styledErrorPrefix() string {
	return styledKey("error:", ansiRed, ansiBold)
}

func styledDetailPrefix(s string) string {
	return styledKey(s, ansiGray)
}

func intToString(v int) string {
	// Small, dependency-free itoa.
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [32]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
