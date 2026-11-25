// SPDX-License-Identifier: AGPL-3.0-or-later
package vault

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EvidenceEntry is a single local-first, hash-chained record stored strictly on
// the user's machine in ~/.goodvibes/vault/evidence.jsonl.
// During passive tracking, only the cryptographic Hash and aggregate metrics are
// sent to the central server. Full redacted entries are only exported if the user
// explicitly generates an AuditBundle for a Hack Club submission.
type EvidenceEntry struct {
	Seq             int64          `json:"seq"`
	Timestamp       int64          `json:"timestamp"`
	Project         string         `json:"project"`
	RepoHash        string         `json:"repo_hash,omitempty"`
	Category        string         `json:"category"` // prompt_typing, prompt_vtt, human_editing, code_review, testing, agent_working
	DurationSec     int            `json:"duration_sec"`
	AgentSec        int            `json:"agent_sec"`
	Harness         string         `json:"harness"`
	Model           string         `json:"model,omitempty"`
	Language        string         `json:"language,omitempty"`
	GitBranch       string         `json:"git_branch,omitempty"`
	GitCommit       string         `json:"git_commit,omitempty"`
	PromptWordCount int            `json:"prompt_word_count,omitempty"`
	PromptTechTerms int            `json:"prompt_tech_terms,omitempty"`
	PromptExcerpt   string         `json:"prompt_excerpt,omitempty"` // scrubbed locally, never sent in normal heartbeats
	StruggleScore   int            `json:"struggle_score"`
	Flags           []string       `json:"flags,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	PrevHash        string         `json:"prev_hash"`
	Hash            string         `json:"hash"`
}

// AuditBundle is a self-contained, verifiable export for external reviewers
// (YSWS authors, Fraud Squad) covering ONLY a single project and time range.
type AuditBundle struct {
	Version         string          `json:"version"`
	GeneratedAt     int64           `json:"generated_at"`
	Project         string          `json:"project"`
	FromTimestamp   int64           `json:"from_timestamp"`
	ToTimestamp     int64           `json:"to_timestamp"`
	EntryCount      int             `json:"entry_count"`
	HumanSeconds    int             `json:"human_seconds"`
	AgentSeconds    int             `json:"agent_seconds"`
	AvgStruggle     float64         `json:"avg_struggle_score"`
	ChainValid      bool            `json:"chain_valid"`
	RootHash        string          `json:"root_hash"`
	HeadHash        string          `json:"head_hash"`
	CategorySummary map[string]int  `json:"category_summary"`
	Entries         []EvidenceEntry `json:"entries"`
}

// Vault manages the local append-only hash-chained evidence log.
type Vault struct {
	mu       sync.Mutex
	path     string
	lastSeq  int64
	lastHash string
}

const genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// Open initializes or loads the local evidence vault from dataDir/vault/evidence.jsonl.
func Open(dataDir string) (*Vault, error) {
	vaultDir := filepath.Join(dataDir, "vault")
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(vaultDir, "evidence.jsonl")
	v := &Vault{
		path:     path,
		lastHash: genesisHash,
	}
	if err := v.loadTail(); err != nil {
		return nil, err
	}
	return v, nil
}

func (v *Vault) loadTail() error {
	f, err := os.Open(v.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e EvidenceEntry
		if err := json.Unmarshal(line, &e); err == nil {
			v.lastSeq = e.Seq
			v.lastHash = e.Hash
		}
	}
	return scanner.Err()
}

// ComputeHash calculates the deterministic SHA-256 hash of an EvidenceEntry
// (excluding the Hash field itself) chained to PrevHash.
func ComputeHash(e EvidenceEntry) string {
	e.Hash = ""
	raw, _ := json.Marshal(e)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Append scrubs sensitive strings, chains the entry to the previous hash,
// and appends it atomically to evidence.jsonl.
func (v *Vault) Append(e EvidenceEntry) (EvidenceEntry, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if e.Timestamp == 0 {
		e.Timestamp = time.Now().Unix()
	}
	if e.PromptExcerpt != "" {
		e.PromptExcerpt = ScrubSecrets(e.PromptExcerpt)
		if len(e.PromptExcerpt) > 280 {
			e.PromptExcerpt = e.PromptExcerpt[:280] + "..."
		}
	}

	e.Seq = v.lastSeq + 1
	e.PrevHash = v.lastHash
	e.Hash = ComputeHash(e)

	line, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	f, err := os.OpenFile(v.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return e, err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return e, err
	}

	v.lastSeq = e.Seq
	v.lastHash = e.Hash
	return e, nil
}

// ExportBundle builds a verifiable AuditBundle for a single project and optional
// time range [fromTS, toTS]. It validates the full underlying hash chain first.
func (v *Vault) ExportBundle(project string, fromTS, toTS int64) (AuditBundle, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	bundle := AuditBundle{
		Version:         "goodvibes-audit-v1",
		GeneratedAt:     time.Now().Unix(),
		Project:         project,
		FromTimestamp:   fromTS,
		ToTimestamp:     toTS,
		ChainValid:      true,
		CategorySummary: make(map[string]int),
	}

	f, err := os.Open(v.path)
	if err != nil {
		if os.IsNotExist(err) {
			return bundle, nil
		}
		return bundle, err
	}
	defer f.Close()

	expectedPrev := genesisHash
	var scoreSum int
	var scoreCount int

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e EvidenceEntry
		if err := json.Unmarshal(line, &e); err != nil {
			bundle.ChainValid = false
			continue
		}
		// Verify global hash chain integrity
		if e.PrevHash != expectedPrev || ComputeHash(e) != e.Hash {
			bundle.ChainValid = false
		}
		expectedPrev = e.Hash

		if project != "" && e.Project != project {
			continue
		}
		if fromTS > 0 && e.Timestamp < fromTS {
			continue
		}
		if toTS > 0 && e.Timestamp > toTS {
			continue
		}

		if bundle.RootHash == "" {
			bundle.RootHash = e.Hash
		}
		bundle.HeadHash = e.Hash
		bundle.Entries = append(bundle.Entries, e)
		bundle.CategorySummary[e.Category] += e.DurationSec
		if e.Category != "agent_working" {
			bundle.HumanSeconds += e.DurationSec
			scoreSum += e.StruggleScore
			scoreCount++
		}
		bundle.AgentSeconds += e.AgentSec
	}
	if err := scanner.Err(); err != nil {
		return bundle, fmt.Errorf("read vault: %w", err)
	}

	bundle.EntryCount = len(bundle.Entries)
	if scoreCount > 0 {
		bundle.AvgStruggle = float64(scoreSum) / float64(scoreCount)
	}
	return bundle, nil
}
