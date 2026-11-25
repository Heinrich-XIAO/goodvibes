// SPDX-License-Identifier: AGPL-3.0-or-later
package controllers

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hackclub/goodvibes/internal/badge"
	"github.com/hackclub/goodvibes/internal/models"
	"github.com/hackclub/goodvibes/internal/services"
)

const lastDay = "9999-12-31"

// rollup is one daily_rollups row: a (UTC day, project, category) bucket.
type rollup struct {
	Day           string `json:"day"`
	Project       string `json:"project"`
	Category      string `json:"category"`
	HumanSeconds  int    `json:"human_seconds"`
	AgentSeconds  int    `json:"agent_seconds"`
	StruggleSum   int    `json:"struggle_sum"`
	StruggleCount int    `json:"struggle_count"`
	Heartbeats    int    `json:"heartbeats"`
}

func utcDay(t time.Time) string { return t.UTC().Format("2006-01-02") }

// rollups returns buckets with day in [sinceDay, untilDay]; userID "" means every user.
func (a *App) rollups(ctx context.Context, userID, sinceDay, untilDay string) []rollup {
	args := map[string]any{"since_day": sinceDay, "until_day": untilDay}
	if userID != "" {
		args["user_id"] = userID
	}
	var rows []rollup
	_ = a.Convex.Query(ctx, "stats:rollups", args, &rows)
	return rows
}

// humanTotals sums billable seconds and the mean struggle score of scored heartbeats.
func humanTotals(rows []rollup) (human, agent, heartbeats int, avgStruggle float64) {
	var sSum, sCount int
	for _, r := range rows {
		human += r.HumanSeconds
		agent += r.AgentSeconds
		heartbeats += r.Heartbeats
		sSum += r.StruggleSum
		sCount += r.StruggleCount
	}
	if sCount > 0 {
		avgStruggle = float64(sSum) / float64(sCount)
	}
	return
}

// HandleUserStats ports Api::V1::StatsController#user_stats + GoodVibes dashboard payload.
func (a *App) HandleUserStats(w http.ResponseWriter, r *http.Request) {
	target := a.LoadUser(r.Context(), r.PathValue("username"))
	if target == nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
		return
	}
	viewer := a.CurrentUser(r)
	if !target.AllowPublicStatsLookup && (viewer == nil || (viewer.ID != target.ID && viewer.AdminLevel == models.AdminLevelDefault)) {
		WriteJSON(w, http.StatusForbidden, map[string]string{"error": "This profile is private"})
		return
	}

	days := 90
	if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && d >= 1 && d <= 365 {
		days = d
	}
	rows := a.rollups(r.Context(), target.ID, utcDay(time.Now().AddDate(0, 0, -days)), lastDay)
	humanSec, agentSec, hbCount, avgStruggle := humanTotals(rows)

	categories := map[string]int{
		"prompt_typing": 0,
		"prompt_vtt":    0,
		"human_editing": 0,
		"coding":        0,
		"code_review":   0,
		"testing":       0,
		"agent_working": 0,
	}
	type projectStat struct {
		Name         string  `json:"name"`
		HumanSeconds int     `json:"human_seconds"`
		AgentSeconds int     `json:"agent_seconds"`
		AvgStruggle  float64 `json:"avg_struggle"`
	}
	byProject := map[string][]rollup{}
	dailyMap := map[string]int{}
	for _, row := range rows {
		if row.Category == "agent_working" {
			categories[row.Category] += row.AgentSeconds
		} else {
			categories[row.Category] += row.HumanSeconds
		}
		if row.Project != "" {
			byProject[row.Project] = append(byProject[row.Project], row)
		}
		dailyMap[row.Day] += row.HumanSeconds
	}
	projects := []projectStat{}
	for name, pr := range byProject {
		h, ag, _, avg := humanTotals(pr)
		projects = append(projects, projectStat{Name: name, HumanSeconds: h, AgentSeconds: ag, AvgStruggle: avg})
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].HumanSeconds > projects[j].HumanSeconds })

	loc, err := time.LoadLocation(target.Timezone)
	if err != nil {
		loc = time.UTC
	}
	currentStreak := models.CalculateStreak(dailyMap, time.Now(), loc)

	var recent []models.Heartbeat
	_ = a.Convex.Query(r.Context(), "heartbeats:recent", map[string]any{"user_id": target.ID, "limit": 25}, &recent)
	type activityItem struct {
		ID            string  `json:"id"`
		Timestamp     float64 `json:"timestamp"`
		Project       string  `json:"project"`
		Category      string  `json:"category"`
		HumanSeconds  int     `json:"human_seconds"`
		AgentSeconds  int     `json:"agent_seconds"`
		Harness       string  `json:"harness"`
		Editor        string  `json:"editor"`
		Model         string  `json:"model"`
		StruggleScore int     `json:"struggle_score"`
	}
	activity := make([]activityItem, 0, len(recent))
	for _, hb := range recent {
		activity = append(activity, activityItem{
			ID: hb.ID, Timestamp: hb.Time, Project: hb.Project, Category: hb.Category,
			HumanSeconds: hb.HumanSeconds, AgentSeconds: hb.AgentSeconds, Harness: hb.Harness,
			Editor: hb.Editor, Model: hb.Model, StruggleScore: hb.StruggleScore,
		})
	}

	pubTrust := target.PublicTrustLevel()
	WriteJSON(w, http.StatusOK, map[string]any{
		"user": target,
		"trust_factor": map[string]any{
			"trust_level": pubTrust.String(),
			"trust_value": int(pubTrust),
		},
		"stats": map[string]any{
			"window_days": days,
			"totals": map[string]any{
				"human_seconds":      humanSec,
				"agent_seconds":      agentSec,
				"avg_struggle_score": math.Round(avgStruggle*10) / 10,
				"heartbeat_count":    hbCount,
			},
			"streaks": map[string]any{
				"current":     currentStreak,
				"longest":     currentStreak,
				"active_days": len(dailyMap),
			},
			"categories":      categories,
			"projects":        projects,
			"recent_activity": activity,
		},
	})
}

// HandleUserSpans ports Api::V1::StatsController#user_spans over the latest 10k human heartbeats.
func (a *App) HandleUserSpans(w http.ResponseWriter, r *http.Request) {
	target := a.LoadUser(r.Context(), r.PathValue("username"))
	if target == nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
		return
	}
	var times []float64
	_ = a.Convex.Query(r.Context(), "heartbeats:humanTimes", map[string]any{"user_id": target.ID, "limit": 10000}, &times)
	WriteJSON(w, http.StatusOK, map[string]any{"spans": models.ToSpans(times, models.HeartbeatTimeoutDuration)})
}

// HandleTrustFactor ports Api::V1::StatsController#trust_factor.
func (a *App) HandleTrustFactor(w http.ResponseWriter, r *http.Request) {
	target := a.LoadUser(r.Context(), r.PathValue("username"))
	if target == nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
		return
	}
	tl := target.PublicTrustLevel()
	WriteJSON(w, http.StatusOK, map[string]any{"trust_level": tl.String(), "trust_value": int(tl)})
}

// HandleUserStream serves real-time SSE heartbeat updates for a user.
func (a *App) HandleUserStream(w http.ResponseWriter, r *http.Request) {
	target := a.LoadUser(r.Context(), r.PathValue("username"))
	viewer := a.CurrentUser(r)
	if target == nil || (!target.AllowPublicStatsLookup && (viewer == nil || viewer.ID != target.ID)) {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found or private"})
		return
	}
	a.Hub.ServeSSE(w, r, target.Username)
}

// HandleBadgeSVG serves GET /api/v1/badge/{username}.svg.
func (a *App) HandleBadgeSVG(w http.ResponseWriter, r *http.Request) {
	target := a.LoadUser(r.Context(), strings.TrimSuffix(r.PathValue("username"), ".svg"))
	if target == nil || !target.AllowPublicStatsLookup {
		http.NotFound(w, r)
		return
	}
	humanSec, _, _, avgStruggle := humanTotals(a.rollups(r.Context(), target.ID, utcDay(time.Now().AddDate(0, 0, -30)), lastDay))

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=60")
	_, _ = w.Write([]byte(badge.RenderStatsSVG(target.Username, humanSec, avgStruggle, 0)))
}

// HandleLeaderboard serves GET /api/v1/leaderboard.
func (a *App) HandleLeaderboard(w http.ResponseWriter, r *http.Request) {
	days := 30
	if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && d >= 1 && d <= 365 {
		days = d
	}
	type leaderRow struct {
		Username     string  `json:"username"`
		DisplayName  string  `json:"display_name"`
		HumanSeconds int     `json:"human_seconds"`
		AgentSeconds int     `json:"agent_seconds"`
		AvgStruggle  float64 `json:"avg_struggle"`
		ProjectCount int     `json:"project_count"`
	}
	users := []leaderRow{}
	_ = a.Convex.Query(r.Context(), "stats:leaderboard", map[string]any{
		"since_day": utcDay(time.Now().AddDate(0, 0, -days)), "limit": 50,
	}, &users)
	WriteJSON(w, http.StatusOK, map[string]any{"window_days": days, "users": users})
}

// HandleSummary ports Api::SummaryController#index (GET /api/summary).
func (a *App) HandleSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to, err := services.DetermineSummaryDateRange(q.Get("interval"), q.Get("range"), q.Get("from"), q.Get("to"), time.Now().UTC())
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid date range"})
		return
	}
	userID := ""
	if uname := q.Get("user"); uname != "" {
		target := a.LoadUser(r.Context(), uname)
		if target == nil {
			WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
			return
		}
		userID = target.ID
	}

	projTotals := map[string]int64{}
	for _, row := range a.rollups(r.Context(), userID, utcDay(from), utcDay(to)) {
		if row.Category == "agent_working" {
			continue
		}
		p := row.Project
		if p == "" {
			p = "Other"
		}
		projTotals[p] += int64(row.HumanSeconds)
	}
	var statsSlice []services.CategoryStat
	for name, sec := range projTotals {
		statsSlice = append(statsSlice, services.CategoryStat{Name: name, TotalSeconds: sec})
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":  q.Get("user"),
		"from":     from.Format(time.RFC3339),
		"to":       to.Format(time.RFC3339),
		"projects": services.FormatCategoryStats(statsSlice),
	})
}
