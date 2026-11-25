// SPDX-License-Identifier: AGPL-3.0-or-later
package services

import (
    "errors"
    "time"
)

// Calculate returns the start (inclusive) and end (exclusive) time range for a given date and period.
// period can be "weekly", "last_7_days" or any other value which defaults to a daily range.
func Calculate(date time.Time, period string) (time.Time, time.Time) {
    // ensure we work with the date at midnight UTC
    d := date.UTC()
    startOfDay := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
    switch period {
    case "weekly":
        end := startOfDay.AddDate(0, 0, 7)
        return startOfDay, end
    case "last_7_days":
        start := startOfDay.AddDate(0, 0, -6)
        // end of the given day (exclusive) -> next midnight
        end := startOfDay.AddDate(0, 0, 1)
        return start, end
    default:
        // daily range – end is next midnight
        return startOfDay, startOfDay.AddDate(0, 0, 1)
    }
}

// NormalizeDate parses a date string (or uses the current date when blank) and optionally shifts it to the start of the week.
func NormalizeDate(dateStr string, period string) (time.Time, error) {
    var dt time.Time
    var err error
    if dateStr == "" {
        dt = time.Now().UTC()
    } else {
        // try to parse as ISO8601 date (YYYY-MM-DD) – ignore time component if present
        layout := "2006-01-02"
        dt, err = time.Parse(layout, dateStr)
        if err != nil {
            // fallback to generic parse
            dt, err = time.Parse(time.RFC3339, dateStr)
        }
        if err != nil {
            return time.Time{}, err
        }
    }
    // strip time to midnight UTC
    dt = time.Date(dt.Year(), dt.Month(), dt.Day(), 0, 0, 0, 0, time.UTC)
    if period == "weekly" {
        // Monday is considered the beginning of the week
        // Weekday(): Sunday=0, Monday=1 … Saturday=6
        // Compute days back to Monday
        daysBack := (int(dt.Weekday()) + 6) % 7
        dt = dt.AddDate(0, 0, -daysBack)
    }
    return dt, nil
}

// DetermineSummaryDateRange mirrors Api::SummaryController#determine_date_range.
// It returns a begin and end time (both inclusive), and an error when parsing fails.
func DetermineSummaryDateRange(interval, rng, fromDate, toDate string, now time.Time) (time.Time, time.Time, error) {
    // Use UTC as the controller does `Time.use_zone("UTC")`.
    now = now.UTC()
    // Explicit range overrides everything.
    if fromDate != "" && toDate != "" {
        from, err1 := time.Parse(time.RFC3339, fromDate)
        to, err2 := time.Parse(time.RFC3339, toDate)
        if err1 != nil || err2 != nil {
            return time.Time{}, time.Time{}, errors.New("invalid from/to dates")
        }
        // beginning_of_day / end_of_day semantics
        from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
        to = time.Date(to.Year(), to.Month(), to.Day(), 23, 59, 59, 0, time.UTC)
        return from, to, nil
    }

    // Allow `range` param to act as interval.
    if interval == "" {
        interval = rng
    }

    switch interval {
    case "today":
        start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
        end := start.AddDate(0, 0, 1).Add(-time.Nanosecond)
        return start, end, nil
    case "yesterday":
        day := now.AddDate(0, 0, -1)
        start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
        end := start.AddDate(0, 0, 1).Add(-time.Nanosecond)
        return start, end, nil
    case "week", "7_days":
        // beginning_of_week .. end_of_week (Monday..Sunday)
        // Align to Monday
        weekday := (int(now.Weekday()) + 6) % 7 // days back to Monday
        weekStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -weekday)
        weekEnd := weekStart.AddDate(0, 0, 7).Add(-time.Nanosecond)
        return weekStart, weekEnd, nil
    case "last_7_days":
        start := now.AddDate(0, 0, -7+1)
        start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
        end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)
        return start, end, nil
    case "month", "30_days":
        start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
        end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
        return start, end, nil
    case "last_30_days":
        start := now.AddDate(0, 0, -30+1)
        start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
        end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)
        return start, end, nil
    case "6_months":
        // beginning_of_month - 5.months .. end_of_month of current month
        start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -5, 0)
        end := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0).Add(-time.Nanosecond)
        return start, end, nil
    case "last_6_months":
        start := now.AddDate(0, -6, 0)
        start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
        end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)
        return start, end, nil
    case "year", "12_months":
        start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
        end := time.Date(now.Year()+1, 1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)
        return start, end, nil
    case "last_12_months", "last_year":
        start := now.AddDate(-1, 0, 0)
        start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
        end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)
        return start, end, nil
    case "any", "all_time", "":
        start := time.Unix(0, 0).UTC()
        end := now
        return start, end, nil
    default:
        // default to today
        start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
        end := start.AddDate(0, 0, 1).Add(-time.Nanosecond)
        return start, end, nil
    }
}
