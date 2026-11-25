// SPDX-License-Identifier: AGPL-3.0-or-later
package services

import (
	"fmt"
	"regexp"
	"strings"
)

type ParseResult struct {
	OS     string
	Editor string
	Err    string
}

// ParseUserAgent parses the wakatime user agent string.
// It returns OS name, editor name and an error string (empty if no error).
func ParseUserAgent(userAgent string) ParseResult {
	// Pattern for wakatime client user agents
	// Example: "wakatime/v1.0.0 (darwin-arm64) go1.0.0 vscode/1.0.0"
	re := regexp.MustCompile(`wakatime\/[^ ]+ \(([^)]+)\)(?: [^ ]+ ([^\/]+)(?:\/([^\/]+))?)?`)
	if matches := re.FindStringSubmatch(userAgent); matches != nil {
		osPart := matches[1]
		// OS may be like "darwin-arm64" – take the part before '-'
		os := strings.Split(osPart, "-")[0]
		editor := matches[2]
		if editor == "" {
			editor = ""
		}
		return ParseResult{OS: os, Editor: editor, Err: ""}
	}

	// Fallback to generic browser-like UA parsing
	// Match "Name/Version" at start
	browserRe := regexp.MustCompile(`^([^\/]+)\/([^\/\s]+)`)
	if m := browserRe.FindStringSubmatch(userAgent); m != nil {
		if strings.Contains(userAgent, "wakatime") {
			// Assume browser extension: take OS token after first space
			parts := strings.Split(userAgent, " ")
			if len(parts) > 1 {
				fullOS := parts[1]
				os := fullOS
				if strings.Contains(fullOS, "_") {
					os = strings.Split(fullOS, "_")[0]
				}
				return ParseResult{OS: os, Editor: strings.ToLower(m[1]), Err: ""}
			}
		}
		return ParseResult{OS: m[1], Editor: m[2], Err: ""}
	}

	// Unable to parse
	return ParseResult{OS: "", Editor: "", Err: "failed to parse user agent string"}
}

// CategoryStat mirrors the Ruby struct used for formatting category statistics.
type CategoryStat struct {
	Name         string  `json:"name"`
	TotalSeconds int64   `json:"total_seconds"`
	Percent      float64 `json:"percent"`
	Digital      string  `json:"digital"`
	Text         string  `json:"text"`
	Hours        int64   `json:"hours"`
	Minutes      int64   `json:"minutes"`
	Seconds      int64   `json:"seconds"`
}

// FormatCategoryStats enriches a slice of CategoryStat with derived fields.
func FormatCategoryStats(stats []CategoryStat) []CategoryStat {
	for i, s := range stats {
		total := s.TotalSeconds
		hours := total / 3600
		minutes := (total % 3600) / 60
		seconds := total % 60
		digital := fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
		text := fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
		stats[i].Hours = hours
		stats[i].Minutes = minutes
		stats[i].Seconds = seconds
		stats[i].Digital = digital
		stats[i].Text = text
	}
	return stats
}
