// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import "testing"

func TestEnumsAndUserHelpers(t *testing.T) {
	if TrustLevelBlue.String() != "blue" || TrustLevelRed != 1 || TrustLevelGreen != 2 || TrustLevelYellow != 3 {
		t.Fatalf("unexpected TrustLevel enum values")
	}
	if AdminLevelSuperadmin != 1 || AdminLevelAdmin != 2 || AdminLevelViewer != 3 {
		t.Fatalf("unexpected AdminLevel enum values")
	}
	if YSWSProgramNeighborhood != 58 {
		t.Fatalf("expected YSWSProgramNeighborhood == 58, got %d", YSWSProgramNeighborhood)
	}

	u := &User{AdminLevel: AdminLevelSuperadmin, TrustLevel: TrustLevelYellow}
	if !u.CanConvictUsers() {
		t.Fatalf("expected superadmin to be able to convict users")
	}
	if u.PublicTrustLevel() != TrustLevelBlue {
		t.Fatalf("expected yellow trust level to mask as blue publicly")
	}
	if StreakDaysFormatted(0) != nil {
		t.Fatalf("expected nil for 0 streak days")
	}
	if s := StreakDaysFormatted(35); s == nil || *s != "30+" {
		t.Fatalf("expected 30+ for 35 streak days")
	}
}

func TestHeartbeatHashingAndAgentExclusion(t *testing.T) {
	if IsBillableCategory("agent_working") {
		t.Fatalf("agent_working MUST NOT be billable")
	}
	if IsBillableCategory("ai coding") {
		t.Fatalf("ai coding MUST NOT be billable")
	}
	if !IsBillableCategory("prompt_typing") || !IsBillableCategory("prompt_vtt") || !IsBillableCategory("coding") {
		t.Fatalf("expected prompt_typing, prompt_vtt, and coding to be billable")
	}

	hb := &Heartbeat{UserID: 1, Project: "cs2-browser", Category: "prompt_typing", Time: 1700000000}
	h1 := GenerateFieldsHash(hb)
	h2 := GenerateFieldsHash(hb)
	if len(h1) != 32 || h1 != h2 {
		t.Fatalf("expected deterministic 32-char MD5 hash, got %q vs %q", h1, h2)
	}
}
