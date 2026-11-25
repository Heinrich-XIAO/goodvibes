// SPDX-License-Identifier: AGPL-3.0-or-later
package audit

import (
	"database/sql"
	"math"
	"strings"
)

type BundleEntry struct {
	Seq             int64    `json:"seq"`
	Timestamp       int64    `json:"timestamp"`
	Project         string   `json:"project"`
	Category        string   `json:"category"`
	DurationSec     int      `json:"duration_sec"`
	AgentSec        int      `json:"agent_sec"`
	Harness         string   `json:"harness"`
	Model           string   `json:"model,omitempty"`
	PromptWordCount int      `json:"prompt_word_count,omitempty"`
	PromptTechTerms int      `json:"prompt_tech_terms,omitempty"`
	PromptExcerpt   string   `json:"prompt_excerpt,omitempty"`
	StruggleScore   int      `json:"struggle_score"`
	Flags           []string `json:"flags,omitempty"`
	PrevHash        string   `json:"prev_hash"`
	Hash            string   `json:"hash"`
}

type Bundle struct {
	Version       string        `json:"version"`
	GeneratedAt   int64         `json:"generated_at"`
	Project       string        `json:"project"`
	ChainValid    *bool         `json:"chain_valid,omitempty"`
	EntryCount    int           `json:"entry_count"`
	HumanSeconds  int           `json:"human_seconds"`
	AgentSeconds  int           `json:"agent_seconds"`
	Entries       []BundleEntry `json:"entries"`
}

type VerificationReport struct {
	Project              string         `json:"project"`
	ChainValid           bool           `json:"chain_valid"`
	TotalEntries         int            `json:"total_entries"`
	MatchedHeartbeats    int            `json:"matched_heartbeats"`
	HeartbeatMatchRate   int            `json:"heartbeat_match_rate"`
	ClaimedHumanSeconds  int            `json:"claimed_human_seconds"`
	ExcludedAgentSeconds int            `json:"excluded_agent_seconds"`
	AvgStruggleScore     float64        `json:"avg_struggle_score"`
	LowEffortEvents      int            `json:"low_effort_events"`
	FlagCounts           map[string]int `json:"flag_counts"`
	TrustScore           int            `json:"trust_score"`
	Recommendation       string         `json:"recommendation"`
}

func isBillable(cat string) bool {
	switch strings.ToLower(strings.TrimSpace(cat)) {
	case "coding", "prompt_typing", "prompt_vtt", "human_editing", "code_review", "testing":
		return true
	default:
		return false
	}
}

// VerifyBundle validates a submitted local-vault AuditBundle against the
// server's recorded heartbeats (`evidence_hash`) for YSWS / Fraud Squad review.
func VerifyBundle(conn *sql.DB, userID int64, project string, bundle Bundle) VerificationReport {
	chainValid := true
	if bundle.ChainValid != nil && !*bundle.ChainValid {
		chainValid = false
	}

	var humanSec, agentSec, lowEffort, scoreSum, scoreCount, matched int
	flagCounts := make(map[string]int)

	for _, e := range bundle.Entries {
		if len(e.Hash) < 32 {
			chainValid = false
		}
		if isBillable(e.Category) {
			humanSec += e.DurationSec
			scoreSum += e.StruggleScore
			scoreCount++
		} else if strings.ToLower(e.Category) == "agent_working" {
			if e.AgentSec > 0 {
				agentSec += e.AgentSec
			} else {
				agentSec += e.DurationSec
			}
		}
		for _, f := range e.Flags {
			flagCounts[f]++
			if f == "low_effort_prompt" || f == "long_unsubmitted_block" || f == "no_verification_yet" {
				lowEffort++
			}
		}
		if e.Hash != "" && conn != nil {
			var id int64
			err := conn.QueryRow(
				"SELECT id FROM heartbeats WHERE user_id = ? AND evidence_hash = ? AND deleted_at IS NULL LIMIT 1",
				userID, e.Hash,
			).Scan(&id)
			if err == nil {
				matched++
			}
		}
	}

	avgStruggle := 0.0
	if scoreCount > 0 {
		avgStruggle = math.Round((float64(scoreSum)/float64(scoreCount))*10) / 10
	}
	matchRate := 0.0
	if len(bundle.Entries) > 0 {
		matchRate = float64(matched) / float64(len(bundle.Entries))
	}
	slopRatio := 0.0
	if scoreCount > 0 {
		slopRatio = float64(lowEffort) / float64(scoreCount)
	}

	trust := 40
	if chainValid {
		trust += 25
	}
	trust += int(math.Round(matchRate * 20))
	trust += int(math.Round((avgStruggle / 100.0) * 15))
	trust -= int(math.Round(slopRatio * 25))
	if trust < 0 {
		trust = 0
	}
	if trust > 100 {
		trust = 100
	}

	rec := "review_for_deflation"
	if trust >= 78 {
		rec = "high_confidence_approve"
	} else if trust >= 55 {
		rec = "manual_spot_check"
	}

	return VerificationReport{
		Project:              project,
		ChainValid:           chainValid,
		TotalEntries:         len(bundle.Entries),
		MatchedHeartbeats:    matched,
		HeartbeatMatchRate:   int(math.Round(matchRate * 100)),
		ClaimedHumanSeconds:  humanSec,
		ExcludedAgentSeconds: agentSec,
		AvgStruggleScore:     avgStruggle,
		LowEffortEvents:      lowEffort,
		FlagCounts:           flagCounts,
		TrustScore:           trust,
		Recommendation:       rec,
	}
}
