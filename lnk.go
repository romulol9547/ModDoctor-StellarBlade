package main

import (
	"os"
	"strings"
	"unicode/utf16"
)

// shortcutTargetsOf reads a .lnk or .url file and returns the paths/links
// written inside it that could be the game.
func shortcutTargetsOf(p string) []string {
	data, err := os.ReadFile(p)
	if err != nil || len(data) > 1<<20 {
		return nil
	}
	var out []string
	if strings.HasSuffix(strings.ToLower(p), ".url") {
		for _, l := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "URL=") {
				out = append(out, strings.TrimSpace(l))
			}
		}
		return out
	}
	// .lnk: collect UTF-16 and plain text runs that look like paths
	for _, s := range textRuns(data) {
		low := strings.ToLower(s)
		if strings.Contains(low, "rungameid/") || (strings.Contains(s, `:\`) && strings.Contains(low, ".exe")) {
			if i := strings.Index(s, ":\\"); i >= 1 && !strings.Contains(low, "rungameid") {
				s = s[i-1:]
			}
			out = append(out, s)
		}
	}
	return out
}

func textRuns(b []byte) []string {
	var out []string
	// UTF-16LE
	var cur []uint16
	flush16 := func() {
		if len(cur) >= 6 {
			out = append(out, string(utf16.Decode(cur)))
		}
		cur = cur[:0]
	}
	for i := 0; i+1 < len(b); i += 2 {
		c := uint16(b[i]) | uint16(b[i+1])<<8
		if c >= 0x20 && c != 0xFFFF {
			cur = append(cur, c)
		} else {
			flush16()
		}
	}
	flush16()
	cur = cur[:0]
	for i := 1; i+1 < len(b); i += 2 { // odd alignment
		c := uint16(b[i]) | uint16(b[i+1])<<8
		if c >= 0x20 && c != 0xFFFF {
			cur = append(cur, c)
		} else {
			flush16()
		}
	}
	flush16()
	// plain bytes (Windows-1252)
	start := -1
	for i := 0; i <= len(b); i++ {
		if i < len(b) && b[i] >= 0x20 && b[i] != 0x7F {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 && i-start >= 6 {
			r := make([]rune, 0, i-start)
			for _, c := range b[start:i] {
				r = append(r, rune(c))
			}
			out = append(out, string(r))
		}
		start = -1
	}
	return out
}
