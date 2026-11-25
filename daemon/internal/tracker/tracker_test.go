// SPDX-License-Identifier: AGPL-3.0-or-later
package tracker

import (
	"testing"

	"github.com/hackclub/goodvibes/daemon/internal/vault"
)

func TestAgentTimeIsStrictlyExcluded(t *testing.T) {
	v, err := vault.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}

	var emitted []Heartbeat
	tr := New(v, 120, func(hb Heartbeat) {
		emitted = append(emitted, hb)
	})

	// 1. Human spends 45s typing a detailed prompt
	tr.ProcessEvent(Event{
		Type:        "prompt_submitted",
		Category:    CatPromptTyping,
		DurationSec: 45,
		Project:     "cs2-browser",
		Harness:     "claude-code",
		Model:       "claude-sonnet-4",
		PromptText:  "Fix the WebGL raycast hitreg bug in src/physics/collision.ts where crouch height offset causes headshots to miss on high latency",
		Keystrokes:  130,
	})

	// 2. Agent spends 180s working (MUST NOT count toward human time!)
	tr.ProcessEvent(Event{
		Type:        "agent_stop",
		Category:    CatAgentWorking,
		DurationSec: 180,
		Project:     "cs2-browser",
		Harness:     "claude-code",
		Model:       "claude-sonnet-4",
	})

	// 3. Human spends 30s dictating next refinement via VTT
	tr.ProcessEvent(Event{
		Type:        "prompt_submitted",
		Category:    CatPromptVTT,
		DurationSec: 30,
		Project:     "cs2-browser",
		Harness:     "whisperflow",
		VTTWords:    42,
		PromptText:  "Now add a debug wireframe overlay for the hitbox buffer so I can verify the matrix transform in the browser",
	})

	st := tr.Status()
	if st.SessionHumanSec != 75 {
		t.Fatalf("expected 75s human active time (45s typing + 30s VTT), got %ds", st.SessionHumanSec)
	}
	if st.SessionAgentSec != 180 {
		t.Fatalf("expected 180s excluded agent time, got %ds", st.SessionAgentSec)
	}
	if len(emitted) != 3 {
		t.Fatalf("expected 3 heartbeats, got %d", len(emitted))
	}
	if emitted[1].HumanSeconds != 0 || emitted[1].AgentSeconds != 180 {
		t.Fatalf("agent_working heartbeat leaked into human time: %+v", emitted[1])
	}
	if emitted[0].StruggleScore < 80 {
		t.Fatalf("expected high productive struggle score for detailed technical prompt, got %d", emitted[0].StruggleScore)
	}
}

func TestLowEffortSlopDetection(t *testing.T) {
	w, tech, low := AnalyzePrompt("fix it")
	if !low || w != 2 || tech != 0 {
		t.Fatalf("expected low effort prompt, got words=%d tech=%d low=%v", w, tech, low)
	}
	score, flags := ScoreProductiveStruggle(CatPromptTyping, 10, w, tech, low, false)
	if score >= 40 {
		t.Fatalf("expected low struggle score for 'fix it', got %d (flags=%v)", score, flags)
	}
}
