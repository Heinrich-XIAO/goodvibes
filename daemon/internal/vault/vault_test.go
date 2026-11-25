// SPDX-License-Identifier: AGPL-3.0-or-later
package vault

import (
	"strings"
	"testing"
)

func TestVaultHashChainAndScrubbing(t *testing.T) {
	v, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open vault: %v", err)
	}

	e1, err := v.Append(EvidenceEntry{
		Project:       "cs2-browser",
		Category:      "prompt_typing",
		DurationSec:   60,
		Harness:       "cursor",
		PromptExcerpt: "Use API_KEY=supersecret123456 and sk-ant-abcdefghijklmnop123456 and email dev@hackclub.com",
		StruggleScore: 85,
	})
	if err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if strings.Contains(e1.PromptExcerpt, "supersecret") || strings.Contains(e1.PromptExcerpt, "sk-ant-") || strings.Contains(e1.PromptExcerpt, "dev@hackclub.com") {
		t.Fatalf("secret scrubber failed: %s", e1.PromptExcerpt)
	}

	e2, err := v.Append(EvidenceEntry{
		Project:       "cs2-browser",
		Category:      "agent_working",
		DurationSec:   120,
		AgentSec:      120,
		Harness:       "cursor",
		StruggleScore: 0,
	})
	if err != nil {
		t.Fatalf("append 2: %v", err)
	}
	if e2.PrevHash != e1.Hash {
		t.Fatalf("hash chain broken: %s != %s", e2.PrevHash, e1.Hash)
	}

	bundle, err := v.ExportBundle("cs2-browser", 0, 0)
	if err != nil {
		t.Fatalf("export bundle: %v", err)
	}
	if !bundle.ChainValid {
		t.Fatalf("expected valid hash chain")
	}
	if bundle.HumanSeconds != 60 || bundle.AgentSeconds != 120 {
		t.Fatalf("unexpected bundle totals: human=%d agent=%d", bundle.HumanSeconds, bundle.AgentSeconds)
	}
}
