// SPDX-License-Identifier: AGPL-3.0-or-later
package gitinfo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// RepoInfo captures lightweight git metadata for a working directory.
type RepoInfo struct {
	RootDir      string `json:"root_dir"`
	ProjectName  string `json:"project_name"`
	RemoteURL    string `json:"remote_url"`
	RepoHash     string `json:"repo_hash"`
	Branch       string `json:"branch"`
	CommitSHA    string `json:"commit_sha"`
	LinesAdded   int    `json:"lines_added"`
	LinesDeleted int    `json:"lines_deleted"`
	FilesChanged int    `json:"files_changed"`
}

func runGit(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

// Detect resolves the git repository for cwd. If cwd is not inside a git repo,
// it falls back to the directory basename so local scratch projects still work.
func Detect(cwd string) RepoInfo {
	if cwd == "" {
		return RepoInfo{ProjectName: "unknown"}
	}
	root := runGit(cwd, "rev-parse", "--show-toplevel")
	if root == "" {
		base := filepath.Base(cwd)
		sum := sha256.Sum256([]byte("local:" + cwd))
		return RepoInfo{
			RootDir:     cwd,
			ProjectName: base,
			RepoHash:    hex.EncodeToString(sum[:8]),
		}
	}

	remote := runGit(root, "config", "--get", "remote.origin.url")
	branch := runGit(root, "rev-parse", "--abbrev-ref", "HEAD")
	commit := runGit(root, "rev-parse", "--short", "HEAD")

	name := filepath.Base(root)
	if remote != "" {
		// Derive canonical repo name from remote URL if possible
		trimmed := strings.TrimSuffix(remote, ".git")
		parts := strings.FieldsFunc(trimmed, func(r rune) bool { return r == '/' || r == ':' })
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			name = parts[len(parts)-1]
		}
	}

	hashInput := remote
	if hashInput == "" {
		hashInput = "local:" + root
	}
	sum := sha256.Sum256([]byte(hashInput))

	added, deleted, files := diffShortStat(root)

	return RepoInfo{
		RootDir:      root,
		ProjectName:  name,
		RemoteURL:    sanitizeRemote(remote),
		RepoHash:     hex.EncodeToString(sum[:8]),
		Branch:       branch,
		CommitSHA:    commit,
		LinesAdded:   added,
		LinesDeleted: deleted,
		FilesChanged: files,
	}
}

func sanitizeRemote(remote string) string {
	// Strip embedded credentials like https://user:token@github.com/...
	if idx := strings.Index(remote, "://"); idx != -1 {
		rest := remote[idx+3:]
		if at := strings.Index(rest, "@"); at != -1 {
			return remote[:idx+3] + rest[at+1:]
		}
	}
	return remote
}

func diffShortStat(root string) (added, deleted, files int) {
	stat := runGit(root, "diff", "--numstat")
	if stat == "" {
		return 0, 0, 0
	}
	for _, line := range strings.Split(stat, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		files++
		if a, err := strconv.Atoi(fields[0]); err == nil {
			added += a
		}
		if d, err := strconv.Atoi(fields[1]); err == nil {
			deleted += d
		}
	}
	return added, deleted, files
}
