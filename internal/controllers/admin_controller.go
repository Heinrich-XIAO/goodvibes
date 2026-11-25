// SPDX-License-Identifier: AGPL-3.0-or-later
package controllers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hackclub/goodvibes/internal/audit"
	"github.com/hackclub/goodvibes/internal/models"
)

// HandleCreateSubmission handles POST /api/v1/submissions (Audit Bundle upload).
func (a *App) HandleCreateSubmission(w http.ResponseWriter, r *http.Request) {
	u := a.CurrentUser(r)
	if u == nil {
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}
	var body struct {
		Project string       `json:"project"`
		Program string       `json:"program"`
		Notes   string       `json:"notes"`
		Bundle  audit.Bundle `json:"bundle"`
	}
	if err := ReadJSON(r, &body, 10<<20); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	proj := strings.TrimSpace(body.Project)
	if proj == "" {
		proj = strings.TrimSpace(body.Bundle.Project)
	}
	if proj == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "project is required"})
		return
	}
	prog := strings.TrimSpace(body.Program)
	if prog == "" {
		prog = "YSWS"
	}

	report := audit.VerifyBundle(a.DB, u.ID, proj, body.Bundle)
	bundleJSON, _ := json.Marshal(body.Bundle)
	verifyJSON, _ := json.Marshal(report)
	now := time.Now().Unix()

	res, err := a.DB.Exec(`
		INSERT INTO audit_submissions (
			user_id, project_name, program, status, claimed_human_seconds, excluded_agent_seconds,
			avg_struggle_score, chain_valid, matched_heartbeats, total_bundle_entries,
			trust_score, notes, bundle_json, verification_json, created_at
		) VALUES (?, ?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, proj, prog, report.ClaimedHumanSeconds, report.ExcludedAgentSeconds,
		report.AvgStruggleScore, boolToInt(report.ChainValid), report.MatchedHeartbeats,
		report.TotalEntries, report.TrustScore, body.Notes, string(bundleJSON), string(verifyJSON), now,
	)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	WriteJSON(w, http.StatusCreated, map[string]any{
		"id":           id,
		"status":       "pending",
		"verification": report,
	})
}

// HandleListSubmissions handles GET /api/v1/submissions.
func (a *App) HandleListSubmissions(w http.ResponseWriter, r *http.Request) {
	subs := []models.AuditSubmission{}
	rows, _ := a.DB.Query(`
		SELECT s.id, s.user_id, u.username, u.display_name, s.project_name, s.program, s.status,
		       s.claimed_human_seconds, s.approved_human_seconds, s.excluded_agent_seconds,
		       s.avg_struggle_score, s.chain_valid, s.matched_heartbeats, s.total_bundle_entries,
		       s.trust_score, s.notes, s.reviewer_notes, s.created_at, s.reviewed_at
		FROM audit_submissions s JOIN users u ON u.id = s.user_id
		ORDER BY s.created_at DESC LIMIT 100`)
	if rows != nil {
		for rows.Next() {
			var s models.AuditSubmission
			var cv int
			if rows.Scan(
				&s.ID, &s.UserID, &s.Username, &s.DisplayName, &s.ProjectName, &s.Program, &s.Status,
				&s.ClaimedHumanSeconds, &s.ApprovedHumanSeconds, &s.ExcludedAgentSeconds,
				&s.AvgStruggleScore, &cv, &s.MatchedHeartbeats, &s.TotalBundleEntries,
				&s.TrustScore, &s.Notes, &s.ReviewerNotes, &s.CreatedAt, &s.ReviewedAt,
			) == nil {
				s.ChainValid = cv != 0
				subs = append(subs, s)
			}
		}
		_ = rows.Close()
	}
	WriteJSON(w, http.StatusOK, map[string]any{"submissions": subs})
}

// HandleGetSubmission handles GET /api/v1/submissions/{id}.
func (a *App) HandleGetSubmission(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var s models.AuditSubmission
	var cv int
	var bJSON, vJSON string
	err = a.DB.QueryRow(`
		SELECT s.id, s.user_id, u.username, u.display_name, s.project_name, s.program, s.status,
		       s.claimed_human_seconds, s.approved_human_seconds, s.excluded_agent_seconds,
		       s.avg_struggle_score, s.chain_valid, s.matched_heartbeats, s.total_bundle_entries,
		       s.trust_score, s.notes, s.reviewer_notes, s.created_at, s.reviewed_at,
		       s.bundle_json, s.verification_json
		FROM audit_submissions s JOIN users u ON u.id = s.user_id
		WHERE s.id = ?`, id).Scan(
		&s.ID, &s.UserID, &s.Username, &s.DisplayName, &s.ProjectName, &s.Program, &s.Status,
		&s.ClaimedHumanSeconds, &s.ApprovedHumanSeconds, &s.ExcludedAgentSeconds,
		&s.AvgStruggleScore, &cv, &s.MatchedHeartbeats, &s.TotalBundleEntries,
		&s.TrustScore, &s.Notes, &s.ReviewerNotes, &s.CreatedAt, &s.ReviewedAt,
		&bJSON, &vJSON,
	)
	if err != nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "Submission not found"})
		return
	}
	s.ChainValid = cv != 0
	var bundle any
	var verify any
	_ = json.Unmarshal([]byte(bJSON), &bundle)
	_ = json.Unmarshal([]byte(vJSON), &verify)

	WriteJSON(w, http.StatusOK, map[string]any{
		"submission": map[string]any{
			"id":                     s.ID,
			"username":               s.Username,
			"display_name":           s.DisplayName,
			"project_name":           s.ProjectName,
			"program":                s.Program,
			"status":                 s.Status,
			"claimed_human_seconds":  s.ClaimedHumanSeconds,
			"approved_human_seconds": s.ApprovedHumanSeconds,
			"excluded_agent_seconds": s.ExcludedAgentSeconds,
			"trust_score":            s.TrustScore,
			"notes":                  s.Notes,
			"reviewer_notes":         s.ReviewerNotes,
			"bundle":                 bundle,
			"verification":           verify,
		},
	})
}

// HandleReviewSubmission handles POST /api/v1/submissions/{id}/review (Fraud Squad / Reviewer).
func (a *App) HandleReviewSubmission(w http.ResponseWriter, r *http.Request) {
	reviewer := a.CurrentUser(r)
	if reviewer == nil || reviewer.AdminLevel == models.AdminLevelDefault {
		WriteJSON(w, http.StatusForbidden, map[string]string{"error": "Reviewer permissions required"})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	var body struct {
		Status               string `json:"status"`
		ApprovedHumanSeconds *int   `json:"approved_human_seconds"`
		ReviewerNotes        string `json:"reviewer_notes"`
	}
	if err := ReadJSON(r, &body, 1<<16); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.Status != "approved" && body.Status != "deflated" && body.Status != "flagged" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be approved, deflated, or flagged"})
		return
	}
	_, err = a.DB.Exec(`
		UPDATE audit_submissions
		SET status = ?, approved_human_seconds = COALESCE(?, claimed_human_seconds),
		    reviewer_notes = ?, reviewed_by = ?, reviewed_at = ?
		WHERE id = ?`,
		body.Status, body.ApprovedHumanSeconds, body.ReviewerNotes, reviewer.ID, time.Now().Unix(), id,
	)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "status": body.Status})
}

// HandleAdminConvict ports POST /api/admin/v1/user/convict (TrustLevel audit log).
func (a *App) HandleAdminConvict(w http.ResponseWriter, r *http.Request) {
	admin := a.CurrentUser(r)
	if admin == nil || !admin.CanConvictUsers() {
		WriteJSON(w, http.StatusForbidden, map[string]string{"error": "Superadmin required"})
		return
	}
	var body struct {
		UserID     int64  `json:"user_id"`
		TrustLevel string `json:"trust_level"`
		Reason     string `json:"reason"`
		Notes      string `json:"notes"`
	}
	if err := ReadJSON(r, &body, 1<<16); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	tl, ok := models.ParseTrustLevel(body.TrustLevel)
	if !ok {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid trust_level"})
		return
	}
	target := a.LoadUserByID(body.UserID)
	if target == nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
		return
	}
	_, _ = a.DB.Exec(`
		INSERT INTO trust_level_audit_logs (user_id, changed_by_id, previous_trust_level, new_trust_level, reason, notes)
		VALUES (?, ?, ?, ?, ?, ?)`,
		target.ID, admin.ID, target.TrustLevel.String(), tl.String(), body.Reason, body.Notes,
	)
	_, _ = a.DB.Exec("UPDATE users SET trust_level = ? WHERE id = ?", int(tl), target.ID)
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "user_id": target.ID, "trust_level": tl.String()})
}
