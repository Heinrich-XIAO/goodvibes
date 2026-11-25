// SPDX-License-Identifier: AGPL-3.0-or-later
package vault

import (
	"regexp"
)

var secretPatterns = []*regexp.Regexp{
	// OpenAI, Anthropic, Stripe, Hack Club / GoodVibes keys
	regexp.MustCompile(`(?i)\b(sk-[a-zA-Z0-9_-]{16,}|sk-ant-[a-zA-Z0-9_-]{16,}|gv_(live|test)_[a-zA-Z0-9_-]{12,})\b`),
	// GitHub tokens
	regexp.MustCompile(`\b(ghp_[a-zA-Z0-9]{20,}|gho_[a-zA-Z0-9]{20,}|github_pat_[a-zA-Z0-9_]{20,})\b`),
	// AWS Access Key IDs
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	// JWTs
	regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}\.[a-zA-Z0-9_-]{10,}\b`),
	// Generic key=value secrets (.env style)
	regexp.MustCompile(`(?i)\b(API_KEY|SECRET_KEY|ACCESS_TOKEN|PASSWORD|PRIVATE_KEY|DATABASE_URL)\s*[:=]\s*[^\s"']{6,}`),
	// Private key blocks
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	// Email addresses
	regexp.MustCompile(`\b[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}\b`),
}

// ScrubSecrets redacts API keys, tokens, credentials, and emails locally
// before anything is stored in the local evidence vault or exported for audit.
func ScrubSecrets(input string) string {
	out := input
	for _, re := range secretPatterns {
		out = re.ReplaceAllString(out, "[REDACTED]")
	}
	return out
}
