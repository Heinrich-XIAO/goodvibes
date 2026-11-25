// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import "time"

type ApiKey struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	Token     string    `json:"token,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type AdminApiKey struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	Name      string     `json:"name"`
	Token     string     `json:"token,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type EmailAddress struct {
	ID     int64  `json:"id"`
	Email  string `json:"email"`
	UserID int64  `json:"user_id"`
	Source int    `json:"source"`
}

type ProjectRepoMapping struct {
	ID           int64  `json:"id"`
	UserID       int64  `json:"user_id"`
	ProjectName  string `json:"project_name"`
	RepoURL      string `json:"repo_url"`
	RepositoryID *int64 `json:"repository_id,omitempty"`
}

type Repository struct {
	ID          int64  `json:"id"`
	URL         string `json:"url"`
	Host        string `json:"host"`
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Stars       int    `json:"stars"`
	Description string `json:"description"`
	Language    string `json:"language"`
	CommitCount int    `json:"commit_count"`
}

type Commit struct {
	SHA          string `json:"sha"`
	UserID       int64  `json:"user_id"`
	RepositoryID *int64 `json:"repository_id,omitempty"`
	GithubRaw    string `json:"github_raw"`
}

type Leaderboard struct {
	ID                   int64              `json:"id"`
	StartDate            string             `json:"start_date"`
	PeriodType           int                `json:"period_type"`
	TimezoneOffset       int                `json:"timezone_offset"`
	FinishedGeneratingAt *time.Time         `json:"finished_generating_at,omitempty"`
	Entries              []LeaderboardEntry `json:"entries,omitempty"`
}

type LeaderboardEntry struct {
	ID            int64  `json:"id"`
	LeaderboardID int64  `json:"leaderboard_id"`
	UserID        int64  `json:"user_id"`
	Username      string `json:"username,omitempty"`
	TotalSeconds  int    `json:"total_seconds"`
	Rank          int    `json:"rank"`
	StreakCount   int    `json:"streak_count"`
}

type TrustLevelAuditLog struct {
	ID                 int64     `json:"id"`
	UserID             int64     `json:"user_id"`
	ChangedByID        int64     `json:"changed_by_id"`
	PreviousTrustLevel string    `json:"previous_trust_level"`
	NewTrustLevel      string    `json:"new_trust_level"`
	Reason             string    `json:"reason"`
	Notes              string    `json:"notes"`
	CreatedAt          time.Time `json:"created_at"`
}

type RawHeartbeatUpload struct {
	ID             int64  `json:"id"`
	RequestHeaders string `json:"request_headers"`
	RequestBody    string `json:"request_body"`
}

type AuditSubmission struct {
	ID                   int64   `json:"id"`
	UserID               int64   `json:"user_id"`
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
