// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"
)

// SourceType enum mirrors app/models/heartbeat.rb.
type SourceType int

const (
	SourceTypeDirectEntry  SourceType = iota // 0
	SourceTypeWakapiImport                   // 1
	SourceTypeTestEntry                      // 2
)

func (s SourceType) String() string {
	switch s {
	case SourceTypeDirectEntry:
		return "direct_entry"
	case SourceTypeWakapiImport:
		return "wakapi_import"
	case SourceTypeTestEntry:
		return "test_entry"
	default:
		return fmt.Sprintf("SourceType(%d)", s)
	}
}

// YSWSProgram enum mirrors all 59 values (0..58) in app/models/heartbeat.rb.
type YSWSProgram int

const (
	YSWSProgramNothing YSWSProgram = iota
	YSWSProgramHighSeas
	YSWSProgramArcade
	YSWSProgramJuice
	YSWSProgramOnboard
	YSWSProgramSprig
	YSWSProgramCider
	YSWSProgramHackpad
	YSWSProgramBobaDrops
	YSWSProgramTheBin
	YSWSProgramBlot
	YSWSProgramInfill
	YSWSProgramScrapyard
	YSWSProgramHackcraftModEdition
	YSWSProgramBrowserBuddy
	YSWSProgramHackaccino
	YSWSProgramCafe
	YSWSProgramLowSkies
	YSWSProgramRaspAPI
	YSWSProgramTerminalCraft
	YSWSProgramNeon
	YSWSProgramJungle
	YSWSProgramCounterspell
	YSWSProgramRiceathon
	YSWSProgramPowerHour
	YSWSProgramScrapyardFlagship
	YSWSProgramTenDaysInPublic
	YSWSProgramBuildYourOwnLLM
	YSWSProgramCargoCultV2
	YSWSProgramSockathon
	YSWSProgramBakebuild
	YSWSProgramMinusTwelve
	YSWSProgramEasel
	YSWSProgramRetrospect
	YSWSProgramCascade
	YSWSProgramTenHoursInPublic
	YSWSProgramSwirl
	YSWSProgramTarot
	YSWSProgramAsylum
	YSWSProgramCargoCult
	YSWSProgramRPG
	YSWSProgramHamClub
	YSWSProgramAnchor
	YSWSProgramDessert
	YSWSProgramWizardOrpheus
	YSWSProgramOnboardLive
	YSWSProgramSayCheese
	YSWSProgramHackapet
	YSWSProgramClubsCompetitions
	YSWSProgramPrintboard
	YSWSProgramBlackBox
	YSWSProgramShipwrecked
	YSWSProgramPizzaGrantYSWS
	YSWSProgramPixeldust
	YSWSProgramHacklet
	YSWSProgramReflow
	YSWSProgramTheJourney
	YSWSProgramVisioneer
	YSWSProgramNeighborhood
)

// Heartbeat mirrors app/models/heartbeat.rb + GoodVibes vibecoding fields.
type Heartbeat struct {
	ID                   int64       `json:"id"`
	UserID               int64       `json:"user_id"`
	Branch               string      `json:"branch"`
	Category             string      `json:"category"`
	Dependencies         string      `json:"dependencies,omitempty"`
	Editor               string      `json:"editor"`
	Entity               string      `json:"entity"`
	Language             string      `json:"language"`
	Machine              string      `json:"machine"`
	OperatingSystem      string      `json:"operating_system"`
	Project              string      `json:"project"`
	Type                 string      `json:"type"`
	UserAgent            string      `json:"user_agent"`
	LineAdditions        int         `json:"line_additions"`
	LineDeletions        int         `json:"line_deletions"`
	Lineno               int         `json:"lineno"`
	Lines                int         `json:"lines"`
	CursorPos            int         `json:"cursorpos"`
	ProjectRootCount     int         `json:"project_root_count"`
	Time                 float64     `json:"time"`
	IsWrite              bool        `json:"is_write"`
	FieldsHash           string      `json:"fields_hash,omitempty"`
	SourceType           SourceType  `json:"source_type"`
	IPAddress            string      `json:"ip_address,omitempty"`
	YSWSProgram          YSWSProgram `json:"ysws_program"`
	RawHeartbeatUploadID *int64      `json:"raw_heartbeat_upload_id,omitempty"`
	// GoodVibes productive-struggle extensions:
	HumanSeconds    int    `json:"human_seconds"`
	AgentSeconds    int    `json:"agent_seconds"`
	Harness         string `json:"harness"`
	Model           string `json:"model"`
	PromptWordCount int    `json:"prompt_word_count"`
	PromptTechTerms int    `json:"prompt_tech_terms"`
	Keystrokes      int    `json:"keystrokes"`
	StruggleScore   int    `json:"struggle_score"`
	FlagsJSON       string `json:"flags_json"`
	EvidenceHash    string `json:"evidence_hash"`
}

// IndexedAttributes mirrors Heartbeat.indexed_attributes.
func IndexedAttributes() []string {
	return []string{
		"user_id", "branch", "category", "dependencies", "editor", "entity",
		"language", "machine", "operating_system", "project", "type", "user_agent",
		"line_additions", "line_deletions", "lineno", "lines", "cursorpos",
		"project_root_count", "time", "is_write",
	}
}

// GenerateFieldsHash mirrors Heartbeat.generate_fields_hash (deterministic MD5).
func GenerateFieldsHash(hb *Heartbeat) string {
	m := map[string]any{
		"user_id":            hb.UserID,
		"branch":             hb.Branch,
		"category":           hb.Category,
		"dependencies":       hb.Dependencies,
		"editor":             hb.Editor,
		"entity":             hb.Entity,
		"language":           hb.Language,
		"machine":            hb.Machine,
		"operating_system":   hb.OperatingSystem,
		"project":            hb.Project,
		"type":               hb.Type,
		"user_agent":         hb.UserAgent,
		"line_additions":     hb.LineAdditions,
		"line_deletions":     hb.LineDeletions,
		"lineno":             hb.Lineno,
		"lines":              hb.Lines,
		"cursorpos":          hb.CursorPos,
		"project_root_count": hb.ProjectRootCount,
		"time":               hb.Time,
		"is_write":           hb.IsWrite,
	}
	b, _ := json.Marshal(m)
	sum := md5.Sum(b)
	return fmt.Sprintf("%x", sum)
}

// IsBillableCategory returns false for agent_working and non-coding categories.
// Core GoodVibes invariant: AI agent execution NEVER counts toward human hours.
func IsBillableCategory(category string) bool {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "agent_working", "ai coding", "browsing", "meeting", "communicating", "idle":
		return false
	case "coding", "prompt_typing", "prompt_vtt", "human_editing", "code_review", "testing":
		return true
	default:
		return false
	}
}
