// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
    "fmt"
    "math"
    "time"
)

const HeartbeatTimeoutDuration = 120 // seconds (2 minutes)
const MaxValidTimestamp = 253402300799.0

type Span struct {
    StartTime float64 `json:"start_time"`
    EndTime   float64 `json:"end_time"`
    Duration  int     `json:"duration"`
}

type TimedPoint struct {
    Time          float64
    Category      string
    GroupKey      string
    HumanSeconds  int
    AgentSeconds  int
}

// ToSpans converts a slice of timestamps (seconds) to spans using a timeout.
func ToSpans(times []float64, timeoutSec int) []Span {
    if len(times) == 0 {
        return []Span{}
    }
    spans := []Span{}
    currentSpanStart := times[0]
    for i, currentTime := range times {
        var nextTime *float64
        if i+1 < len(times) {
            nt := times[i+1]
            nextTime = &nt
        }
        // Close span if no next time or gap exceeds timeout
        if nextTime == nil || (*nextTime-currentTime) > float64(timeoutSec) {
            // Duration from start to current time (no gap added)
            totalDuration := int(math.Round(currentTime - currentSpanStart))
            if totalDuration > 0 {
                spans = append(spans, Span{StartTime: currentSpanStart, EndTime: currentTime, Duration: totalDuration})
            }
            // Start new span at next time if it exists
            if nextTime != nil {
                currentSpanStart = *nextTime
            }
        }
    }
    return spans
}

// DurationSeconds returns the summed duration between consecutive timestamps,
// capping each gap at timeoutSec and ignoring invalid timestamps.
func DurationSeconds(times []float64, timeoutSec int) int {
    valid := []float64{}
    for _, t := range times {
        if t >= 0 && t <= MaxValidTimestamp {
            valid = append(valid, t)
        }
    }
    if len(valid) < 2 {
        return 0
    }
    total := 0
    for i := 1; i < len(valid); i++ {
        diff := valid[i] - valid[i-1]
        if diff > float64(timeoutSec) {
            diff = float64(timeoutSec)
        }
        total += int(math.Round(diff))
    }
    return total
}

// DurationSecondsGrouped groups points by GroupKey, skips "agent_working" category,
// and returns the summed duration per group using the same timeout logic.
func DurationSecondsGrouped(points []TimedPoint, timeoutSec int) map[string]int {
    groups := map[string][]float64{}
    for _, p := range points {
        if p.Category == "agent_working" {
            continue
        }
        if p.Time < 0 || p.Time > MaxValidTimestamp {
            continue
        }
        groups[p.GroupKey] = append(groups[p.GroupKey], p.Time)
    }
    result := map[string]int{}
    for key, times := range groups {
        result[key] = DurationSeconds(times, timeoutSec)
    }
    return result
}

// DurationSecondsBoundaryAware calculates duration within a time window, excluding certain categories
// and including the heartbeat immediately before the start time.
func DurationSecondsBoundaryAware(allPoints []TimedPoint, startTime, endTime float64, timeoutSec int) int {
    excluded := map[string]bool{"browsing": true, "ai coding": true, "meeting": true, "communicating": true}
    valid := []float64{}
    for _, p := range allPoints {
        if p.Time < 0 || p.Time > MaxValidTimestamp {
            continue
        }
        if excluded[p.Category] {
            continue
        }
        valid = append(valid, p.Time)
    }
    if len(valid) == 0 {
        return 0
    }
    var boundary *float64
    for _, t := range valid {
        if t < startTime {
            if boundary == nil || t > *boundary {
                tt := t
                boundary = &tt
            }
        }
    }
    window := []float64{}
    for _, t := range valid {
        if t >= startTime && t <= endTime {
            window = append(window, t)
        }
    }
    if len(window) == 0 {
        return 0
    }
    total := 0
    var prev float64
    prevSet := false
    if boundary != nil {
        prev = *boundary
        prevSet = true
    }
    for _, t := range window {
        if !prevSet {
            prev = t
            prevSet = true
            continue
        }
        diff := t - prev
        if diff > float64(timeoutSec) {
            diff = float64(timeoutSec)
        }
        total += int(math.Round(diff))
        prev = t
    }
    return total
}

// DurationFormatted formats seconds as HH:MM:SS.
func DurationFormatted(seconds int) string {
    hours := seconds / 3600
    minutes := (seconds % 3600) / 60
    remaining := seconds % 60
    return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, remaining)
}

// DurationSimple returns a concise human‑readable string.
func DurationSimple(seconds int) string {
    hours := seconds / 3600
    minutes := (seconds % 3600) / 60
    if hours > 1 {
        return fmt.Sprintf("%d hrs", hours)
    } else if hours == 1 {
        return "1 hr"
    } else if minutes > 0 {
        return fmt.Sprintf("%d min", minutes)
    }
    return "0 min"
}

// CalculateStreak computes the daily coding streak.
// dailySeconds maps a date string (YYYY‑MM‑DD) to total seconds coded that day.
func CalculateStreak(dailySeconds map[string]int, now time.Time, loc *time.Location) int {
    const minPerDay = 15 * 60
    today := now.In(loc).Format("2006-01-02")
    streak := 0
    if secs, ok := dailySeconds[today]; ok && secs >= minPerDay {
        streak = 1
    }
    for i := 1; ; i++ {
        d := now.AddDate(0, 0, -i).In(loc).Format("2006-01-02")
        secs, ok := dailySeconds[d]
        if !ok || secs < minPerDay {
            break
        }
        streak++
    }
    return streak
}
