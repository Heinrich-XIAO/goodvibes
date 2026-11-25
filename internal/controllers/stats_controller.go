// SPDX-License-Identifier: AGPL-3.0-or-later
package controllers

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hackclub/goodvibes/internal/badge"
	"github.com/hackclub/goodvibes/internal/models"
	"github.com/hackclub/goodvibes/internal/services"
)

// HandleUserStats ports Api::V1::StatsController#user_stats + GoodVibes dashboard payload.
func (a *App) HandleUserStats(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	target := a.LoadUserByUsernameOrID(username)
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
	since := float64(time.Now().AddDate(0, 0, -days).Unix())

	var humanSec, agentSec, hbCount int
	var avgStruggle float64
	_ = a.DB.QueryRow(`
		SELECT COALESCE(SUM(human_seconds),0),
		       COALESCE(SUM(agent_seconds),0),
		       COALESCE(AVG(CASE WHEN human_seconds > 0 THEN struggle_score END),0),
		       COUNT(*)
		FROM heartbeats
		WHERE user_id = ? AND deleted_at IS NULL AND time >= ?`,
		target.ID, since,
	).Scan(&humanSec, &agentSec, &avgStruggle, &hbCount)

	categories := map[string]int{
		"prompt_typing": 0,
		"prompt_vtt":    0,
		"human_editing": 0,
		"coding":        0,
		"code_review":   0,
		"testing":       0,
		"agent_working": 0,
	}
	catRows, _ := a.DB.Query(`
		SELECT LOWER(category), COALESCE(SUM(human_seconds),0), COALESCE(SUM(agent_seconds),0)
		FROM heartbeats
		WHERE user_id = ? AND deleted_at IS NULL AND time >= ?
		GROUP BY LOWER(category)`, target.ID, since)
	if catRows != nil {
		for catRows.Next() {
			var c string
			var h, ag int
			if catRows.Scan(&c, &h, &ag) == nil {
				if c == "agent_working" {
					categories["agent_working"] += ag
				} else {
					categories[c] += h
				}
			}
		}
		_ = catRows.Close()
	}

	type projectStat struct {
		Name         string  `json:"name"`
		HumanSeconds int     `json:"human_seconds"`
		AgentSeconds int     `json:"agent_seconds"`
		AvgStruggle  float64 `json:"avg_struggle"`
	}
	var projects []projectStat
	pRows, _ := a.DB.Query(`
		SELECT project, COALESCE(SUM(human_seconds),0), COALESCE(SUM(agent_seconds),0),
		       COALESCE(AVG(CASE WHEN human_seconds > 0 THEN struggle_score END),0)
		FROM heartbeats
		WHERE user_id = ? AND deleted_at IS NULL AND time >= ? AND project != ''
		GROUP BY project
		ORDER BY SUM(human_seconds) DESC`, target.ID, since)
	if pRows != nil {
		for pRows.Next() {
			var ps projectStat
			if pRows.Scan(&ps.Name, &ps.HumanSeconds, &ps.AgentSeconds, &ps.AvgStruggle) == nil {
				projects = append(projects, ps)
			}
		}
		_ = pRows.Close()
	}

	dailyMap := make(map[string]int)
	dRows, _ := a.DB.Query(`
		SELECT date(CAST(time AS INTEGER), 'unixepoch'), COALESCE(SUM(human_seconds),0)
		FROM heartbeats
		WHERE user_id = ? AND deleted_at IS NULL AND time >= ?
		GROUP BY 1`, target.ID, since)
	if dRows != nil {
		for dRows.Next() {
			var day string
			var sec int
			if dRows.Scan(&day, &sec) == nil {
				dailyMap[day] = sec
			}
		}
		_ = dRows.Close()
	}
	loc, err := time.LoadLocation(target.Timezone)
	if err != nil {
		loc = time.UTC
	}
	currentStreak := models.CalculateStreak(dailyMap, time.Now(), loc)

	type activityItem struct {
		ID            int64   `json:"id"`
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
	var recent []activityItem
	rRows, _ := a.DB.Query(`
		SELECT id, time, project, category, human_seconds, agent_seconds, harness, editor, model, struggle_score
		FROM heartbeats
		WHERE user_id = ? AND deleted_at IS NULL
		ORDER BY time DESC, id DESC LIMIT 25`, target.ID)
	if rRows != nil {
		for rRows.Next() {
			var it activityItem
			if rRows.Scan(&it.ID, &it.Timestamp, &it.Project, &it.Category, &it.HumanSeconds, &it.AgentSeconds, &it.Harness, &it.Editor, &it.Model, &it.StruggleScore) == nil {
				recent = append(recent, it)
			}
		}
		_ = rRows.Close()
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
			"recent_activity": recent,
		},
	})
}

// HandleUserSpans ports Api::V1::StatsController#user_spans.
func (a *App) HandleUserSpans(w http.ResponseWriter, r *http.Request) {
	target := a.LoadUserByUsernameOrID(r.PathValue("username"))
	if target == nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
		return
	}
	rows, _ := a.DB.Query(`
		SELECT time FROM heartbeats
		WHERE user_id = ? AND deleted_at IS NULL AND LOWER(category) != 'agent_working'
		ORDER BY time ASC`, target.ID)
	var times []float64
	if rows != nil {
		for rows.Next() {
			var ts float64
			if rows.Scan(&ts) == nil {
				times = append(times, ts)
			}
		}
		_ = rows.Close()
	}
	WriteJSON(w, http.StatusOK, map[string]any{"spans": models.ToSpans(times, models.HeartbeatTimeoutDuration)})
}

// HandleTrustFactor ports Api::V1::StatsController#trust_factor.
func (a *App) HandleTrustFactor(w http.ResponseWriter, r *http.Request) {
	target := a.LoadUserByUsernameOrID(r.PathValue("username"))
	if target == nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
		return
	}
	tl := target.PublicTrustLevel()
	WriteJSON(w, http.StatusOK, map[string]any{"trust_level": tl.String(), "trust_value": int(tl)})
}

// HandleUserStream serves real-time SSE heartbeat updates for a user.
func (a *App) HandleUserStream(w http.ResponseWriter, r *http.Request) {
	target := a.LoadUserByUsernameOrID(r.PathValue("username"))
	viewer := a.CurrentUser(r)
	if target == nil || (!target.AllowPublicStatsLookup && (viewer == nil || viewer.ID != target.ID)) {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found or private"})
		return
	}
	a.Hub.ServeSSE(w, r, target.Username)
}

// HandleBadgeSVG serves GET /api/v1/badge/{username}.svg.
func (a *App) HandleBadgeSVG(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSuffix(r.PathValue("username"), ".svg")
	target := a.LoadUserByUsernameOrID(raw)
	if target == nil || !target.AllowPublicStatsLookup {
		http.NotFound(w, r)
		return
	}
	since := float64(time.Now().AddDate(0, 0, -30).Unix())
	var humanSec int
	var avgStruggle float64
	_ = a.DB.QueryRow(`
		SELECT COALESCE(SUM(human_seconds),0),
		       COALESCE(AVG(CASE WHEN human_seconds > 0 THEN struggle_score END),0)
		FROM heartbeats WHERE user_id = ? AND deleted_at IS NULL AND time >= ?`,
		target.ID, since,
	).Scan(&humanSec, &avgStruggle)

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
	since := float64(time.Now().AddDate(0, 0, -days).Unix())

	type leaderRow struct {
		Username     string  `json:"username"`
		DisplayName  string  `json:"display_name"`
		HumanSeconds int     `json:"human_seconds"`
		AgentSeconds int     `json:"agent_seconds"`
		AvgStruggle  float64 `json:"avg_struggle"`
		ProjectCount int     `json:"project_count"`
	}
	users := []leaderRow{}
	rows, _ := a.DB.Query(`
		SELECT u.username, u.display_name,
		       COALESCE(SUM(h.human_seconds),0),
		       COALESCE(SUM(h.agent_seconds),0),
		       COALESCE(AVG(CASE WHEN h.human_seconds > 0 THEN h.struggle_score END),0),
		       COUNT(DISTINCT CASE WHEN h.project != '' THEN h.project END)
		FROM users u
		LEFT JOIN heartbeats h ON h.user_id = u.id AND h.deleted_at IS NULL AND h.time >= ?
		WHERE u.allow_public_stats_lookup = 1
		GROUP BY u.id
		ORDER BY SUM(h.human_seconds) DESC, u.created_at DESC
		LIMIT 50`, since)
	if rows != nil {
		for rows.Next() {
			var lr leaderRow
			if rows.Scan(&lr.Username, &lr.DisplayName, &lr.HumanSeconds, &lr.AgentSeconds, &lr.AvgStruggle, &lr.ProjectCount) == nil {
				users = append(users, lr)
			}
		}
		_ = rows.Close()
	}
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
	var userID int64
	if uname := q.Get("user"); uname != "" {
		target := a.LoadUserByUsernameOrID(uname)
		if target == nil {
			WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
			return
		}
		userID = target.ID
	}

	projTotals := make(map[string]int)
	rows, _ := a.DB.Query(`
		SELECT project, COALESCE(SUM(human_seconds),0)
		FROM heartbeats
		WHERE deleted_at IS NULL AND time >= ? AND time <= ? AND LOWER(category) != 'agent_working'
		  AND (? = 0 OR user_id = ?)
		GROUP BY project`, float64(from.Unix()), float64(to.Unix()), userID, userID)
	if rows != nil {
		for rows.Next() {
			var p string
			var sec int
			if rows.Scan(&p, &sec) == nil {
				if p == "" {
					p = "Other"
				}
				projTotals[p] = sec
			}
		}
		_ = rows.Close()
	}

	var statsSlice []services.CategoryStat
	for name, sec := range projTotals {
		statsSlice = append(statsSlice, services.CategoryStat{Name: name, TotalSeconds: int64(sec)})
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":  q.Get("user"),
		"from":     from.Format(time.RFC3339),
		"to":       to.Format(time.RFC3339),
		"projects": services.FormatCategoryStats(statsSlice),
	})
}
