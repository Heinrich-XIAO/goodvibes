// SPDX-License-Identifier: AGPL-3.0-or-later
package tracker

import (
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hackclub/goodvibes/daemon/internal/gitinfo"
	"github.com/hackclub/goodvibes/daemon/internal/vault"
)

// ActivityCategory classifies what is happening at any moment.
// CRITICAL RULE: "agent_working" NEVER counts toward productive struggle hours.
// Only genuine human active categories count:
//   - prompt_typing : actively typing out a prompt to an AI harness
//   - prompt_vtt    : actively speaking a prompt via Voice-to-Text (VTT)
//   - human_editing : directly writing/editing project code
//   - code_review   : inspecting diffs, reading outputs, debugging
//   - testing       : running builds, tests, or playing/testing the app
type ActivityCategory string

const (
	CatIdle         ActivityCategory = "idle"
	CatPromptTyping ActivityCategory = "prompt_typing"
	CatPromptVTT    ActivityCategory = "prompt_vtt"
	CatHumanEditing ActivityCategory = "human_editing"
	CatCodeReview   ActivityCategory = "code_review"
	CatTesting      ActivityCategory = "testing"
	CatAgentWorking ActivityCategory = "agent_working"
)

// IsBillableHumanTime returns true ONLY for active human effort categories.
func IsBillableHumanTime(cat ActivityCategory) bool {
	switch cat {
	case CatPromptTyping, CatPromptVTT, CatHumanEditing, CatCodeReview, CatTesting:
		return true
	default:
		return false
	}
}

// Event is emitted by any harness plugin (Claude Code, Cursor, VS Code,
// Codex CLI, VTT adapter, browser extension, or `goodvibes wrap`).
type Event struct {
	Type          string           `json:"type"` // "start", "pulse", "stop", "prompt_submitted", "agent_start", "agent_stop"
	Category      ActivityCategory `json:"category"`
	Timestamp     int64            `json:"timestamp,omitempty"`
	DurationSec   int              `json:"duration_sec,omitempty"` // explicit duration when known (e.g. prompt composition timer)
	Cwd           string           `json:"cwd,omitempty"`
	Project       string           `json:"project,omitempty"`
	Harness       string           `json:"harness,omitempty"`
	Model         string           `json:"model,omitempty"`
	Language      string           `json:"language,omitempty"`
	PromptText    string           `json:"prompt_text,omitempty"` // stays LOCAL ONLY! Used only to compute structural metrics & redacted vault entry
	Keystrokes    int              `json:"keystrokes,omitempty"`
	VTTWords      int              `json:"vtt_words,omitempty"`
	FilesTouched  int              `json:"files_touched,omitempty"`
	TestPassed    *bool            `json:"test_passed,omitempty"`
}

// Heartbeat is the privacy-safe payload transmitted to the central server.
// Notice: NO raw prompt text, NO source code, NO screenshots.
type Heartbeat struct {
	Timestamp       int64            `json:"timestamp"`
	Project         string           `json:"project"`
	RepoHash        string           `json:"repo_hash,omitempty"`
	RemoteURL       string           `json:"remote_url,omitempty"`
	GitBranch       string           `json:"git_branch,omitempty"`
	GitCommit       string           `json:"git_commit,omitempty"`
	Category        ActivityCategory `json:"category"`
	HumanSeconds    int              `json:"human_seconds"` // 0 when category == agent_working
	AgentSeconds    int              `json:"agent_seconds"` // tracked separately for transparency
	Harness         string           `json:"harness"`
	Model           string           `json:"model,omitempty"`
	Language        string           `json:"language,omitempty"`
	PromptWordCount int              `json:"prompt_word_count,omitempty"`
	PromptTechTerms int              `json:"prompt_tech_terms,omitempty"`
	Keystrokes      int              `json:"keystrokes,omitempty"`
	LinesAdded      int              `json:"lines_added,omitempty"`
	LinesDeleted    int              `json:"lines_deleted,omitempty"`
	StruggleScore   int              `json:"struggle_score"`
	Flags           []string         `json:"flags,omitempty"`
	EvidenceHash    string           `json:"evidence_hash"` // binds to local vault entry
}

// StatusSnapshot reports live daemon state for `goodvibes status`.
type StatusSnapshot struct {
	CurrentState     ActivityCategory `json:"current_state"`
	CurrentProject   string           `json:"current_project"`
	CurrentHarness   string           `json:"current_harness"`
	SessionHumanSec  int              `json:"session_human_sec"`
	SessionAgentSec  int              `json:"session_agent_sec"`
	HeartbeatsEmitted int             `json:"heartbeats_emitted"`
	LastEventAt      int64            `json:"last_event_at"`
}

// Tracker maintains the real-time state machine, computes productive struggle
// scores, writes to the local vault, and produces privacy-safe heartbeats.
type Tracker struct {
	mu                 sync.Mutex
	vault              *vault.Vault
	onHeartbeat        func(Heartbeat)
	idleTimeoutSec     int
	state              ActivityCategory
	stateStartedAt     int64
	lastEventAt        int64
	activeProject      string
	activeCwd          string
	activeHarness      string
	activeModel        string
	activeLanguage     string
	sessionHumanSec    int
	sessionAgentSec    int
	recentPromptCount  int
	recentReviewEvents int
	heartbeatsEmitted  int
}

func New(v *vault.Vault, idleTimeoutSec int, onHeartbeat func(Heartbeat)) *Tracker {
	if idleTimeoutSec <= 0 {
		idleTimeoutSec = 120
	}
	return &Tracker{
		vault:          v,
		onHeartbeat:    onHeartbeat,
		idleTimeoutSec: idleTimeoutSec,
		state:          CatIdle,
	}
}

var lowEffortPrompts = regexp.MustCompile(`(?i)^(fix|fix it|continue|go|yes|ok|okay|do it|try again|more|next|pls|please fix|make it work|keep going)[\.\!\?]*$`)
var techTermPattern = regexp.MustCompile(`(?i)\b(error|bug|stack|trace|shader|webgl|canvas|physics|hitreg|latency|websocket|sqlite|api|async|mutex|refactor|test|assert|crash|null|undefined|memory|fps|buffer|query|schema|auth|token|css|layout|responsive|component|hook|state|diff|commit|branch|build|compile|panic|exception|segfault|404|500|cors|http|tcp|udp|json|regex|algorithm|render|frame|collision|raycast|vector|matrix)\b`)

// AnalyzePrompt computes privacy-preserving structural metrics locally.
func AnalyzePrompt(text string) (wordCount int, techTerms int, isLowEffort bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, 0, false
	}
	words := strings.Fields(trimmed)
	wordCount = len(words)
	matches := techTermPattern.FindAllString(trimmed, -1)
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		seen[strings.ToLower(m)] = struct{}{}
	}
	techTerms = len(seen)
	if lowEffortPrompts.MatchString(trimmed) || (wordCount <= 3 && techTerms == 0) {
		isLowEffort = true
	}
	return wordCount, techTerms, isLowEffort
}

// ScoreProductiveStruggle returns a transparent 0-100 score and any anti-slop
// or positive craft flags.
func ScoreProductiveStruggle(cat ActivityCategory, durationSec int, wordCount int, techTerms int, isLowEffort bool, hasVerification bool) (int, []string) {
	if cat == CatAgentWorking || cat == CatIdle {
		return 0, []string{"agent_excluded"}
	}

	score := 65
	var flags []string

	switch cat {
	case CatPromptTyping, CatPromptVTT:
		if isLowEffort {
			score = 22
			flags = append(flags, "low_effort_prompt")
		} else {
			if wordCount >= 12 {
				score += 12
				flags = append(flags, "detailed_spec")
			}
			if wordCount >= 35 {
				score += 8
			}
			if techTerms >= 2 {
				score += 10
				flags = append(flags, "technical_precision")
			}
			if cat == CatPromptVTT {
				flags = append(flags, "voice_dictation")
			}
		}
		if durationSec > 600 {
			// Single uninterrupted typing block >10m without submit is suspicious
			score -= 15
			flags = append(flags, "long_unsubmitted_block")
		}
	case CatHumanEditing:
		score = 88
		flags = append(flags, "hands_on_coding")
	case CatCodeReview:
		score = 84
		flags = append(flags, "active_review")
	case CatTesting:
		score = 86
		flags = append(flags, "verification_loop")
	}

	if hasVerification && score > 30 {
		score += 5
	}
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	return score, flags
}

// ProcessEvent ingests an event from any harness plugin and updates the state machine.
func (t *Tracker) ProcessEvent(ev Event) *Heartbeat {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := ev.Timestamp
	if now == 0 {
		now = time.Now().Unix()
	}

	// Normalize special shorthand event types
	switch ev.Type {
	case "agent_start":
		ev.Category = CatAgentWorking
	case "agent_stop":
		// Agent finished; record explicit duration if provided, else flush elapsed span,
		// then transition to idle until the human acts again.
		var hb *Heartbeat
		if ev.DurationSec > 0 {
			repo := gitinfo.Detect(t.activeCwd)
			proj := ev.Project
			if proj == "" {
				proj = t.activeProject
			}
			if proj == "" {
				proj = repo.ProjectName
			}
			recorded := t.recordSpanLocked(now, proj, repo, CatAgentWorking, ev.DurationSec, ev)
			hb = &recorded
		} else {
			hb = t.flushLocked(now, ev)
		}
		t.state = CatIdle
		t.stateStartedAt = now
		t.lastEventAt = now
		return hb
	}

	if ev.Cwd != "" {
		t.activeCwd = ev.Cwd
	}
	if ev.Harness != "" {
		t.activeHarness = ev.Harness
	}
	if ev.Model != "" {
		t.activeModel = ev.Model
	}
	if ev.Language != "" {
		t.activeLanguage = ev.Language
	}

	repo := gitinfo.Detect(t.activeCwd)
	proj := ev.Project
	if proj == "" {
		proj = repo.ProjectName
	}
	if proj == "" {
		proj = t.activeProject
	}
	if proj == "" {
		proj = "unnamed-project"
	}

	// Direct duration event (e.g., `goodvibes wrap` or Claude Code hook reporting exact
	// prompt typing/VTT time before Enter, or an explicit agent_working span).
	if ev.DurationSec > 0 && ev.Category != "" && ev.Category != CatIdle {
		t.activeProject = proj
		t.lastEventAt = now
		hb := t.recordSpanLocked(now, proj, repo, ev.Category, ev.DurationSec, ev)
		// If a prompt was just submitted, immediately switch state to agent_working
		// so we DO NOT count the time the AI spends generating!
		if ev.Type == "prompt_submitted" {
			t.state = CatAgentWorking
			t.stateStartedAt = now
		}
		return &hb
	}

	// State-transition / pulse model for passive editor/VTT/browser listeners
	var emitted *Heartbeat
	if t.state != CatIdle && t.stateStartedAt > 0 {
		elapsed := int(now - t.stateStartedAt)
		// Clamp idle gaps so stepping away never inflates human time
		if t.lastEventAt > 0 && int(now-t.lastEventAt) > t.idleTimeoutSec && t.state != CatAgentWorking {
			elapsed = int(t.lastEventAt - t.stateStartedAt)
			if elapsed < 1 {
				elapsed = 1
			}
		}
		// Flush if switching state, switching project, or elapsed >= 30s
		if (ev.Category != "" && ev.Category != t.state) || (proj != t.activeProject && t.activeProject != "") || elapsed >= 30 || ev.Type == "stop" {
			if elapsed > 0 {
				hb := t.recordSpanLocked(now, t.activeProject, repo, t.state, elapsed, ev)
				emitted = &hb
				t.stateStartedAt = now
			}
		}
	}

	t.activeProject = proj
	t.lastEventAt = now

	if ev.Type == "stop" {
		t.state = CatIdle
		t.stateStartedAt = 0
	} else if ev.Category != "" {
		if t.state != ev.Category {
			t.state = ev.Category
			t.stateStartedAt = now
		}
	}

	return emitted
}

func (t *Tracker) flushLocked(now int64, ev Event) *Heartbeat {
	if t.state == CatIdle || t.stateStartedAt == 0 {
		return nil
	}
	elapsed := int(now - t.stateStartedAt)
	if elapsed <= 0 {
		return nil
	}
	repo := gitinfo.Detect(t.activeCwd)
	proj := t.activeProject
	if proj == "" {
		proj = repo.ProjectName
	}
	hb := t.recordSpanLocked(now, proj, repo, t.state, elapsed, ev)
	return &hb
}

func (t *Tracker) recordSpanLocked(now int64, project string, repo gitinfo.RepoInfo, cat ActivityCategory, durationSec int, ev Event) Heartbeat {
	if durationSec < 1 {
		durationSec = 1
	}
	// Cap any single span to 15 minutes to prevent sleep/wake clock jumps
	if durationSec > 900 {
		durationSec = 900
	}

	wordCount, techTerms, isLowEffort := AnalyzePrompt(ev.PromptText)
	if ev.VTTWords > wordCount {
		wordCount = ev.VTTWords
	}

	if cat == CatHumanEditing || cat == CatCodeReview || cat == CatTesting {
		t.recentReviewEvents++
	} else if cat == CatPromptTyping || cat == CatPromptVTT {
		t.recentPromptCount++
	}

	hasVerification := t.recentReviewEvents > 0
	score, flags := ScoreProductiveStruggle(cat, durationSec, wordCount, techTerms, isLowEffort, hasVerification)
	if (cat == CatPromptTyping || cat == CatPromptVTT) && t.recentPromptCount >= 5 && t.recentReviewEvents == 0 {
		flags = append(flags, "no_verification_yet")
		if score > 40 {
			score -= 12
		}
	}

	humanSec := 0
	agentSec := 0
	if IsBillableHumanTime(cat) {
		humanSec = durationSec
		t.sessionHumanSec += humanSec
	} else if cat == CatAgentWorking {
		agentSec = durationSec
		t.sessionAgentSec += agentSec
	}

	harness := t.activeHarness
	if ev.Harness != "" {
		harness = ev.Harness
	}
	if harness == "" {
		harness = "generic"
	}

	model := t.activeModel
	if ev.Model != "" {
		model = ev.Model
	}

	lang := t.activeLanguage
	if ev.Language != "" {
		lang = ev.Language
	}

	// Write full (secret-scrubbed) evidence entry strictly to the local vault
	evidenceHash := ""
	if t.vault != nil {
		entry, err := t.vault.Append(vault.EvidenceEntry{
			Timestamp:       now,
			Project:         project,
			RepoHash:        repo.RepoHash,
			Category:        string(cat),
			DurationSec:     durationSec,
			AgentSec:        agentSec,
			Harness:         harness,
			Model:           model,
			Language:        lang,
			GitBranch:       repo.Branch,
			GitCommit:       repo.CommitSHA,
			PromptWordCount: wordCount,
			PromptTechTerms: techTerms,
			PromptExcerpt:   ev.PromptText, // scrubbed by vault.Append, stays local!
			StruggleScore:   score,
			Flags:           flags,
		})
		if err == nil {
			evidenceHash = entry.Hash
		}
	}

	hb := Heartbeat{
		Timestamp:       now,
		Project:         project,
		RepoHash:        repo.RepoHash,
		RemoteURL:       repo.RemoteURL,
		GitBranch:       repo.Branch,
		GitCommit:       repo.CommitSHA,
		Category:        cat,
		HumanSeconds:    humanSec,
		AgentSeconds:    agentSec,
		Harness:         harness,
		Model:           model,
		Language:        lang,
		PromptWordCount: wordCount,
		PromptTechTerms: techTerms,
		Keystrokes:      ev.Keystrokes,
		LinesAdded:      repo.LinesAdded,
		LinesDeleted:    repo.LinesDeleted,
		StruggleScore:   score,
		Flags:           flags,
		EvidenceHash:    evidenceHash,
	}

	t.heartbeatsEmitted++
	if t.onHeartbeat != nil {
		t.onHeartbeat(hb)
	}
	return hb
}

// TickIdle checks if the human has gone idle and flushes any pending human span.
func (t *Tracker) TickIdle(now int64) *Heartbeat {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == CatIdle || t.state == CatAgentWorking {
		return nil
	}
	if t.lastEventAt > 0 && int(now-t.lastEventAt) >= t.idleTimeoutSec {
		hb := t.flushLocked(t.lastEventAt, Event{})
		t.state = CatIdle
		t.stateStartedAt = 0
		return hb
	}
	return nil
}

// Status returns a snapshot of the tracker's live state.
func (t *Tracker) Status() StatusSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return StatusSnapshot{
		CurrentState:      t.state,
		CurrentProject:    t.activeProject,
		CurrentHarness:    t.activeHarness,
		SessionHumanSec:   t.sessionHumanSec,
		SessionAgentSec:   t.sessionAgentSec,
		HeartbeatsEmitted: t.heartbeatsEmitted,
		LastEventAt:       t.lastEventAt,
	}
}
