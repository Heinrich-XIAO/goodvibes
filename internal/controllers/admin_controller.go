// SPDX-License-Identifier: AGPL-3.0-or-later
package controllers

import (
	"encoding/json"
	"net/http"
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

	var matched int
	if err := a.Convex.Query(r.Context(), "heartbeats:countEvidence", map[string]any{
		"user_id": u.ID, "hashes": audit.EvidenceHashes(body.Bundle),
	}, &matched); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	report := audit.VerifyBundle(proj, body.Bundle, matched)
	bundleJSON, _ := json.Marshal(body.Bundle)
	verifyJSON, _ := json.Marshal(report)

	var id string
	err := a.Convex.Mutation(r.Context(), "submissions:create", map[string]any{
		"user_id":                u.ID,
		"project_name":           proj,
		"program":                prog,
		"claimed_human_seconds":  report.ClaimedHumanSeconds,
		"excluded_agent_seconds": report.ExcludedAgentSeconds,
		"avg_struggle_score":     report.AvgStruggleScore,
		"chain_valid":            report.ChainValid,
		"matched_heartbeats":     report.MatchedHeartbeats,
		"total_bundle_entries":   report.TotalEntries,
		"trust_score":            report.TrustScore,
		"notes":                  body.Notes,
		"bundle_json":            string(bundleJSON),
		"verification_json":      string(verifyJSON),
	}, &id)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{
		"id":           id,
		"status":       "pending",
		"verification": report,
	})
}

// HandleListSubmissions handles GET /api/v1/submissions.
func (a *App) HandleListSubmissions(w http.ResponseWriter, r *http.Request) {
	subs := []models.AuditSubmission{}
	if err := a.Convex.Query(r.Context(), "submissions:list", map[string]any{"limit": 100}, &subs); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"submissions": subs})
}

// HandleGetSubmission handles GET /api/v1/submissions/{id}.
func (a *App) HandleGetSubmission(w http.ResponseWriter, r *http.Request) {
	var s *struct {
		models.AuditSubmission
		BundleJSON       string `json:"bundle_json"`
		VerificationJSON string `json:"verification_json"`
	}
	if err := a.Convex.Query(r.Context(), "submissions:get", map[string]any{"id": r.PathValue("id")}, &s); err != nil || s == nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "Submission not found"})
		return
	}
	var bundle any
	var verify any
	_ = json.Unmarshal([]byte(s.BundleJSON), &bundle)
	_ = json.Unmarshal([]byte(s.VerificationJSON), &verify)

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
	id := r.PathValue("id")
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
	args := map[string]any{
		"id":             id,
		"status":         body.Status,
		"reviewer_notes": body.ReviewerNotes,
		"reviewed_by":    reviewer.ID,
		"reviewed_at":    time.Now().Unix(),
	}
	if body.ApprovedHumanSeconds != nil {
		args["approved_human_seconds"] = *body.ApprovedHumanSeconds
	}
	var found bool
	if err := a.Convex.Mutation(r.Context(), "submissions:review", args, &found); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "Submission not found"})
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
		UserID     string `json:"user_id"`
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
	target := a.LoadUser(r.Context(), body.UserID)
	if target == nil {
		WriteJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
		return
	}
	if err := a.Convex.Mutation(r.Context(), "users:setTrustLevel", map[string]any{
		"user_id":              target.ID,
		"changed_by_id":        admin.ID,
		"previous_trust_level": target.TrustLevel.String(),
		"new_trust_level":      tl.String(),
		"trust_level":          int(tl),
		"reason":               body.Reason,
		"notes":                body.Notes,
	}, nil); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "user_id": target.ID, "trust_level": tl.String()})
}
