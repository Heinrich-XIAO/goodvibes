// SPDX-License-Identifier: AGPL-3.0-or-later
package badge

import (
	"fmt"
	"html"
	"math"
)

func formatHours(sec int) string {
	h := float64(sec) / 3600.0
	if h < 1 {
		return fmt.Sprintf("%dm", int(math.Round(float64(sec)/60.0)))
	}
	return fmt.Sprintf("%.1fh", h)
}

// RenderStatsSVG generates an embeddable README SVG badge for a public user profile.
func RenderStatsSVG(username string, humanSeconds int, avgStruggle float64, streakDays int) string {
	hrs := formatHours(humanSeconds)
	value := fmt.Sprintf("%s active · %d craft · %dd streak", hrs, int(math.Round(avgStruggle)), streakDays)
	leftW := 86
	rightW := int(math.Max(185, float64(len(value))*6.8+20))
	totalW := leftW + rightW

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="24" role="img" aria-label="%s %s">
  <linearGradient id="gv-bg" x2="0" y2="100%%">
    <stop offset="0" stop-color="#ec3750"/>
    <stop offset="1" stop-color="#c9243d"/>
  </linearGradient>
  <clipPath id="gv-clip"><rect width="%d" height="24" rx="5" fill="#fff"/></clipPath>
  <g clip-path="url(#gv-clip)">
    <rect width="%d" height="24" fill="url(#gv-bg)"/>
    <rect x="%d" width="%d" height="24" fill="#18181b"/>
  </g>
  <g fill="#fff" font-family="ui-sans-serif, system-ui, -apple-system, sans-serif" font-size="11.5" font-weight="600">
    <text x="%d" y="16" text-anchor="middle">goodvibes</text>
    <text x="%d" y="16" text-anchor="middle" fill="#f4f4f5" font-weight="500">%s</text>
  </g>
</svg>`,
		totalW, html.EscapeString(username), html.EscapeString(value),
		totalW, leftW, leftW, rightW, leftW/2, leftW+rightW/2, html.EscapeString(value))
}
