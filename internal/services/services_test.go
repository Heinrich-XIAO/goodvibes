// SPDX-License-Identifier: AGPL-3.0-or-later
package services

import (
	"reflect"
	"testing"
	"time"
)

func TestCalculate(t *testing.T) {
	base := time.Date(2023, 5, 10, 12, 0, 0, 0, time.UTC)
	start, end := Calculate(base, "weekly")
	if !start.Equal(time.Date(2023, 5, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("weekly start incorrect: %v", start)
	}
	if !end.Equal(time.Date(2023, 5, 17, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("weekly end incorrect: %v", end)
	}
	start, end = Calculate(base, "last_7_days")
	if !start.Equal(time.Date(2023, 5, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("last_7_days start incorrect: %v", start)
	}
	if !end.Equal(time.Date(2023, 5, 11, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("last_7_days end incorrect: %v", end)
	}
	start, end = Calculate(base, "daily")
	if !start.Equal(time.Date(2023, 5, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("daily start incorrect: %v", start)
	}
	if !end.Equal(time.Date(2023, 5, 11, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("daily end incorrect: %v", end)
	}
}

func TestNormalizeDate(t *testing.T) {
	// blank date defaults to today (approximate check)
	today, _ := NormalizeDate("", "")
	now := time.Now().UTC()
	if today.Year() != now.Year() || today.Month() != now.Month() || today.Day() != now.Day() {
		t.Fatalf("blank date not set to today: %v vs %v", today, now)
	}
	// specific date
	d, err := NormalizeDate("2023-01-15", "")
	if err != nil {
		t.Fatalf("error parsing date: %v", err)
	}
	if d != time.Date(2023, 1, 15, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("unexpected date: %v", d)
	}
	// weekly shift to Monday
	d, _ = NormalizeDate("2023-05-10", "weekly")          // Wednesday
	if d != time.Date(2023, 5, 8, 0, 0, 0, 0, time.UTC) { // Monday
		t.Fatalf("weekly normalization failed: %v", d)
	}
}

func TestDetermineSummaryDateRange(t *testing.T) {
	now := time.Date(2023, 5, 10, 15, 0, 0, 0, time.UTC)
	// today
	from, to, err := DetermineSummaryDateRange("today", "", "", "", now)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !from.Equal(time.Date(2023, 5, 10, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("today from wrong: %v", from)
	}
	if !to.Equal(time.Date(2023, 5, 10, 23, 59, 59, 999999999, time.UTC)) {
		t.Fatalf("today to wrong: %v", to)
	}
	// last_7_days
	from, to, _ = DetermineSummaryDateRange("last_7_days", "", "", "", now)
	if !from.Equal(time.Date(2023, 5, 4, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("last_7_days from wrong: %v", from)
	}
	if !to.Truncate(time.Second).Equal(time.Date(2023, 5, 10, 23, 59, 59, 0, time.UTC)) {
		t.Fatalf("last_7_days to wrong: %v", to)
	}
	// any
	from, to, _ = DetermineSummaryDateRange("any", "", "", "", now)
	if !from.Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("any from wrong: %v", from)
	}
	if !to.Equal(now) {
		t.Fatalf("any to wrong: %v", to)
	}
}

func TestParseUserAgent(t *testing.T) {
	cases := []struct {
		ua      string
		expOS   string
		expEdit string
		expErr  string
	}{
		{"wakatime/v1.0.0 (darwin-arm64) go1.0.0 vscode/1.0.0", "darwin", "vscode", ""},
		{"wakatime/v1.0.0 (darwin-arm64) go1.0.0 github-desktop/1.0.0", "darwin", "github-desktop", ""},
		{"wakatime/v1.0.0 (darwin-arm64) go1.0.0 figma/1.0.0", "darwin", "figma", ""},
		{"wakatime/v1.0.0 (darwin-arm64) go1.0.0 terminal/1.0.0", "darwin", "terminal", ""},
		{"wakatime/v1.0.0 (darwin-arm64) go1.0.0 vim/1.0.0", "darwin", "vim", ""},
		{"wakatime/v1.0.0 (windows-x86_64) go1.0.0 vscode/1.0.0", "windows", "vscode", ""},
		{"wakatime/v1.0.0 (darwin-arm64) go1.0.0 cursor/1.0.0", "darwin", "cursor", ""},
		{"Firefox/139.0 linux_x86-64 firefox-wakatime/4.1.0", "linux", "firefox", ""},
		{"invalid-user-agent", "", "", "failed to parse user agent string"},
	}
	for _, c := range cases {
		res := ParseUserAgent(c.ua)
		if res.OS != c.expOS || res.Editor != c.expEdit || res.Err != c.expErr {
			t.Fatalf("ParseUserAgent(%q) = %+v, expected OS=%s Editor=%s Err=%s", c.ua, res, c.expOS, c.expEdit, c.expErr)
		}
	}
}

func TestFormatCategoryStats(t *testing.T) {
	input := []CategoryStat{{Name: "test", TotalSeconds: 3661}}
	out := FormatCategoryStats(input)
	// Percent is not set by the function; ignore in comparison
	if out[0].Hours != 1 || out[0].Minutes != 1 || out[0].Seconds != 1 {
		t.Fatalf("unexpected time breakdown: %+v", out[0])
	}
	if out[0].Digital != "01:01:01" || out[0].Text != "1h 1m 1s" {
		t.Fatalf("unexpected formatted strings: %+v", out[0])
	}
	// Ensure other fields unchanged
	if out[0].Name != "test" || out[0].TotalSeconds != 3661 {
		t.Fatalf("unexpected other fields: %+v", out[0])
	}
	// Compare slice equality ignoring Percent (zero value)
	if !reflect.DeepEqual(out, []CategoryStat{{Name: "test", TotalSeconds: 3661, Percent: 0, Digital: "01:01:01", Text: "1h 1m 1s", Hours: 1, Minutes: 1, Seconds: 1}}) {
		t.Fatalf("unexpected result: %+v", out)
	}
}
