// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"reflect"
	"testing"
	"time"
)

func TestToSpans(t *testing.T) {
	times := []float64{10, 20, 150, 170}
	timeout := 60 // seconds
	spans := ToSpans(times, timeout)
	expected := []Span{{StartTime: 10, EndTime: 20, Duration: 10}, {StartTime: 150, EndTime: 170, Duration: 20}}
	if !reflect.DeepEqual(spans, expected) {
		t.Fatalf("expected %v, got %v", expected, spans)
	}
}

func TestDurationSeconds(t *testing.T) {
	times := []float64{0, 30, 200, 500}
	timeout := 120
	// diffs: 30, 120 (capped), 120 (capped) => sum 270
	got := DurationSeconds(times, timeout)
	if got != 270 {
		t.Fatalf("expected 270, got %d", got)
	}
}

func TestDurationSecondsGrouped(t *testing.T) {
	points := []TimedPoint{{Time: 0, Category: "coding", GroupKey: "a"}, {Time: 50, Category: "agent_working", GroupKey: "a"}, {Time: 200, Category: "coding", GroupKey: "b"}}
	timeout := 120
	res := DurationSecondsGrouped(points, timeout)
	if res["a"] != 0 { // only one valid point, no diff
		t.Fatalf("group a expected 0, got %d", res["a"])
	}
	if res["b"] != 0 { // single point, no diff
		t.Fatalf("group b expected 0, got %d", res["b"])
	}
}

func TestDurationSecondsBoundaryAware(t *testing.T) {
	points := []TimedPoint{{Time: 90, Category: "coding"}, {Time: 150, Category: "browsing"}, {Time: 300, Category: "coding"}}
	timeout := 200
	// Exclude browsing, include boundary before start (none here). Within window 100..350:
	// valid times: 90 (before), 300 (in window). Boundary heartbeat = 90, then diff = 300-90=210 capped to 200 => 200
	got := DurationSecondsBoundaryAware(points, 100, 350, timeout)
	if got != 200 {
		t.Fatalf("expected 200, got %d", got)
	}
}

func TestDurationFormatted(t *testing.T) {
	if got := DurationFormatted(3661); got != "01:01:01" {
		t.Fatalf("expected 01:01:01, got %s", got)
	}
}

func TestDurationSimple(t *testing.T) {
	cases := []struct {
		secs int
		want string
	}{{7200, "2 hrs"}, {3600, "1 hr"}, {600, "10 min"}, {0, "0 min"}}
	for _, c := range cases {
		if got := DurationSimple(c.secs); got != c.want {
			t.Fatalf("seconds %d: expected %s, got %s", c.secs, c.want, got)
		}
	}
}

func TestCalculateStreak(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Date(2023, 3, 5, 12, 0, 0, 0, loc)
	daily := map[string]int{"2023-03-05": 1000, "2023-03-04": 900, "2023-03-03": 800, "2023-03-02": 1000}
	// 2023-03-05 meets min (>=900), streak should count 5,4,2 (3 fails) => streak 2 (today + yesterday)
	got := CalculateStreak(daily, now, loc)
	if got != 2 {
		t.Fatalf("expected streak 2, got %d", got)
	}
}
