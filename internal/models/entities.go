// SPDX-License-Identifier: AGPL-3.0-or-later
package models

type AuditSubmission struct {
	ID                   string  `json:"id"`
	UserID               string  `json:"user_id"`
	Username             string  `json:"username,omitempty"`
	DisplayName          string  `json:"display_name,omitempty"`
	ProjectName          string  `json:"project_name"`
	Program              string  `json:"program"`
	Status               string  `json:"status"`
	ClaimedHumanSeconds  int     `json:"claimed_human_seconds"`
	ApprovedHumanSeconds *int    `json:"approved_human_seconds,omitempty"`
	ExcludedAgentSeconds int     `json:"excluded_agent_seconds"`
	AvgStruggleScore     float64 `json:"avg_struggle_score"`
	ChainValid           bool    `json:"chain_valid"`
	MatchedHeartbeats    int     `json:"matched_heartbeats"`
	TotalBundleEntries   int     `json:"total_bundle_entries"`
	TrustScore           int     `json:"trust_score"`
	Notes                string  `json:"notes"`
	ReviewerNotes        string  `json:"reviewer_notes"`
	CreatedAt            int64   `json:"created_at"`
	ReviewedAt           *int64  `json:"reviewed_at,omitempty"`
}
