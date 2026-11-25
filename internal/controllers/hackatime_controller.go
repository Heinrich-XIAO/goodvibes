// SPDX-License-Identifier: AGPL-3.0-or-later
package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/hackclub/goodvibes/internal/models"
	"github.com/hackclub/goodvibes/internal/services"
)

// heartbeatInput supports BOTH WakaTime/Hackatime payloads and GoodVibes daemon payloads.
type heartbeatInput struct {
	Branch           string  `json:"branch"`
	GitBranch        string  `json:"git_branch"`
	Category         string  `json:"category"`
	Editor           string  `json:"editor"`
	Entity           string  `json:"entity"`
	Language         string  `json:"language"`
	Machine          string  `json:"machine"`
	OperatingSystem  string  `json:"operating_system"`
	Project          string  `json:"project"`
	Type             string  `json:"type"`
	UserAgent        string  `json:"user_agent"`
	Plugin           string  `json:"plugin"`
	LineAdditions    int     `json:"line_additions"`
	LinesAdded       int     `json:"lines_added"`
	LineDeletions    int     `json:"line_deletions"`
	LinesDeleted     int     `json:"lines_deleted"`
	Lineno           int     `json:"lineno"`
	Lines            int     `json:"lines"`
	CursorPos        int     `json:"cursorpos"`
	ProjectRootCount int     `json:"project_root_count"`
	Time             float64 `json:"time"`
	Timestamp        float64 `json:"timestamp"`
	IsWrite          bool    `json:"is_write"`
	// GoodVibes vibecoding fields:
	HumanSeconds    int      `json:"human_seconds"`
	AgentSeconds    int      `json:"agent_seconds"`
	Harness         string   `json:"harness"`
	Model           string   `json:"model"`
	PromptWordCount int      `json:"prompt_word_count"`
	PromptTechTerms int      `json:"prompt_tech_terms"`
	Keystrokes      int      `json:"keystrokes"`
	StruggleScore   int      `json:"struggle_score"`
	Flags           []string `json:"flags"`
	EvidenceHash    string   `json:"evidence_hash"`
}

// HandlePushHeartbeats ports Api::Hackatime::V1::HackatimeController#push_heartbeats
// AND GoodVibes's POST /api/v1/heartbeats.
// Strictly enforces: agent_working NEVER counts as human_seconds!
func (a *App) HandlePushHeartbeats(w http.ResponseWriter, r *http.Request) {
	u := a.CurrentUser(r)
	if u == nil {
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}

	// Accept single object, JSON array (WakaTime .bulk), or {"heartbeats": [...]}
	var raw json.RawMessage
	if err := ReadJSON(r, &raw, 4<<20); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}

	var inputs []heartbeatInput
	trimmed := bytesTrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &inputs); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	} else {
		var wrapper struct {
			Heartbeats []heartbeatInput `json:"heartbeats"`
		}
		if err := json.Unmarshal(trimmed, &wrapper); err == nil && len(wrapper.Heartbeats) > 0 {
			inputs = wrapper.Heartbeats
		} else {
			var single heartbeatInput
			if err := json.Unmarshal(trimmed, &single); err != nil {
				WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			inputs = []heartbeatInput{single}
		}
	}

	if len(inputs) == 0 || len(inputs) > 500 {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "provide 1..500 heartbeats"})
		return
	}

	isBulk := strings.HasSuffix(r.URL.Path, ".bulk")
	var responses []any
	var lastHB *models.Heartbeat

	hbs := make([]*models.Heartbeat, len(inputs))
	for i, in := range inputs {
		hbs[i] = a.normalizeHeartbeat(u.ID, r, in)
	}
	var results []struct {
		ID        string `json:"id"`
		Duplicate bool   `json:"duplicate"`
	}
	if err := a.Convex.Mutation(r.Context(), "heartbeats:insertMany", map[string]any{"heartbeats": hbs}, &results); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for i, res := range results {
		hbs[i].ID = res.ID
		lastHB = hbs[i]
		responses = append(responses, []any{hbs[i], http.StatusCreated})
	}

	if lastHB != nil && a.Hub != nil {
		a.Hub.Publish(u.Username, "heartbeat", map[string]any{
			"username":  u.Username,
			"heartbeat": lastHB,
			"accepted":  len(inputs),
		})
	}

	if isBulk {
		WriteJSON(w, http.StatusCreated, map[string]any{"responses": responses})
		return
	}
	WriteJSON(w, http.StatusAccepted, map[string]any{"ok": true, "accepted": len(inputs), "heartbeat": lastHB})
}

func (a *App) normalizeHeartbeat(userID string, r *http.Request, in heartbeatInput) *models.Heartbeat {
	ts := in.Time
	if ts <= 0 {
		ts = in.Timestamp
	}
	if ts <= 0 {
		ts = float64(time.Now().Unix())
	}

	ua := in.UserAgent
	if ua == "" {
		ua = in.Plugin
	}
	if ua == "" {
		ua = r.UserAgent()
	}
	parsedUA := services.ParseUserAgent(ua)
	osName, editorName := parsedUA.OS, parsedUA.Editor
	if in.OperatingSystem != "" {
		osName = in.OperatingSystem
	}
	if in.Editor != "" {
		editorName = in.Editor
	}

	cat := strings.ToLower(strings.TrimSpace(in.Category))
	if cat == "" {
		cat = "coding"
	}

	branch := in.Branch
	if branch == "" {
		branch = in.GitBranch
	}
	additions := in.LineAdditions
	if additions == 0 {
		additions = in.LinesAdded
	}
	deletions := in.LineDeletions
	if deletions == 0 {
		deletions = in.LinesDeleted
	}

	sourceType := models.SourceTypeDirectEntry
	if in.Entity == "test.txt" {
		sourceType = models.SourceTypeTestEntry
	}

	// CRITICAL INVARIANT: agent_working NEVER counts as human_seconds!
	humanSec := 0
	agentSec := clampInt(in.AgentSeconds, 0, 3600)
	struggleScore := 0
	if models.IsBillableCategory(cat) {
		humanSec = clampInt(in.HumanSeconds, 0, 900)
		struggleScore = clampInt(in.StruggleScore, 0, 100)
		if struggleScore == 0 && humanSec > 0 {
			struggleScore = 65
		}
	} else {
		humanSec = 0
		struggleScore = 0
	}

	flagsJSON := "[]"
	if len(in.Flags) > 0 {
		if b, err := json.Marshal(in.Flags); err == nil {
			flagsJSON = string(b)
		}
	}

	proj := strings.TrimSpace(in.Project)
	if proj == "" {
		proj = "unnamed-project"
	}
	harness := strings.TrimSpace(in.Harness)
	if harness == "" {
		harness = editorName
	}

	hb := &models.Heartbeat{
		UserID:           userID,
		Branch:           branch,
		Category:         cat,
		Editor:           editorName,
		Entity:           in.Entity,
		Language:         in.Language,
		Machine:          in.Machine,
		OperatingSystem:  osName,
		Project:          proj,
		Type:             in.Type,
		UserAgent:        ua,
		LineAdditions:    additions,
		LineDeletions:    deletions,
		Lineno:           in.Lineno,
		Lines:            in.Lines,
		CursorPos:        in.CursorPos,
		ProjectRootCount: in.ProjectRootCount,
		Time:             ts,
		IsWrite:          in.IsWrite,
		SourceType:       sourceType,
		IPAddress:        r.RemoteAddr,
		HumanSeconds:     humanSec,
		AgentSeconds:     agentSec,
		Harness:          harness,
		Model:            in.Model,
		PromptWordCount:  in.PromptWordCount,
		PromptTechTerms:  in.PromptTechTerms,
		Keystrokes:       in.Keystrokes,
		StruggleScore:    struggleScore,
		FlagsJSON:        flagsJSON,
		EvidenceHash:     in.EvidenceHash,
	}
	hb.FieldsHash = models.GenerateFieldsHash(hb)
	return hb
}

// HandleStatusBarToday ports Api::Hackatime::V1::HackatimeController#status_bar_today.
func (a *App) HandleStatusBarToday(w http.ResponseWriter, r *http.Request) {
	u := a.CurrentUser(r)
	if u == nil {
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	totalSec := a.humanSecondsSince(r, u.ID, time.Now())

	WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"grand_total": map[string]any{
				"text":          u.FormatExtensionText(totalSec),
				"total_seconds": totalSec,
			},
		},
	})
}

// HandleStatsLast7Days ports Api::Hackatime::V1::HackatimeController#stats_last_7_days.
func (a *App) HandleStatsLast7Days(w http.ResponseWriter, r *http.Request) {
	u := a.CurrentUser(r)
	if u == nil {
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
		return
	}
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -7)
	totalSec := a.humanSecondsSince(r, u.ID, start)

	hours := totalSec / 3600
	mins := (totalSec % 3600) / 60
	WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"username":             u.Username,
			"user_id":              u.SlackUID,
			"start":                start.Format(time.RFC3339),
			"end":                  now.Format(time.RFC3339),
			"status":               "ok",
			"total_seconds":        totalSec,
			"range":                "last_7_days",
			"human_readable_range": "Last 7 Days",
			"human_readable_total": fmt.Sprintf("%d hrs %d mins", hours, mins),
		},
	})
}

// humanSecondsSince sums non-agent human seconds from the UTC day of since onward.
func (a *App) humanSecondsSince(r *http.Request, userID string, since time.Time) int {
	total := 0
	for _, row := range a.rollups(r.Context(), userID, utcDay(since), lastDay) {
		if row.Category != "agent_working" {
			total += row.HumanSeconds
		}
	}
	return total
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
