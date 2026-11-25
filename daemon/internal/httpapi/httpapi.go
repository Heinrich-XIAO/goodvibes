// SPDX-License-Identifier: AGPL-3.0-or-later
package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/hackclub/goodvibes/daemon/internal/sync"
	"github.com/hackclub/goodvibes/daemon/internal/tracker"
)

// NewHandler creates the localhost-only plugin event receiver API.
// Any harness (Claude Code, Cursor, VS Code, VTT tools, browser extensions)
// sends lightweight JSON events to http://127.0.0.1:42420/v1/events.
func NewHandler(t *tracker.Tracker, s *syncer.Client) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		addCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"service": "goodvibes-daemon",
			"pending": s.PendingCount(),
			"status":  t.Status(),
		})
	})

	mux.HandleFunc("/v1/events", func(w http.ResponseWriter, r *http.Request) {
		addCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}
		var ev tracker.Event
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&ev); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
			return
		}
		hb := t.ProcessEvent(ev)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"heartbeat": hb,
			"status":    t.Status(),
		})
	})

	// Generic T3 Code Lifecycle Hook receiver (configured in ~/.t3/hooks.json).
	// Translates T3 Code's generic lifecycle events into GoodVibes productive-struggle spans:
	//   - composer.submitted -> prompt_typing / prompt_vtt (billable human time)
	//   - turn.started       -> agent_working (0s human time)
	//   - turn.completed     -> agent_stop (agent_seconds excluded from human hours)
	mux.HandleFunc("/v1/t3-hook", func(w http.ResponseWriter, r *http.Request) {
		addCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
			return
		}
		var hook struct {
			Type              string `json:"type"`
			Timestamp         int64  `json:"timestamp"`
			WorkspaceRoot     string `json:"workspaceRoot"`
			Provider          string `json:"provider"`
			Model             string `json:"model"`
			DurationMs        int    `json:"durationMs"`
			ActiveDurationSec int    `json:"activeDurationSec"`
			InputMode         string `json:"inputMode"`
			Keystrokes        int    `json:"keystrokes"`
			WordCount         int    `json:"wordCount"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&hook); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
			return
		}

		var ev tracker.Event
		ev.Timestamp = hook.Timestamp
		ev.Cwd = hook.WorkspaceRoot
		ev.Harness = "t3-code"
		if hook.Provider != "" {
			ev.Harness = "t3-code:" + hook.Provider
		}
		ev.Model = hook.Model

		switch hook.Type {
		case "composer.submitted":
			ev.Type = "prompt_submitted"
			if hook.InputMode == "vtt" {
				ev.Category = tracker.CatPromptVTT
				ev.VTTWords = hook.WordCount
			} else {
				ev.Category = tracker.CatPromptTyping
				ev.Keystrokes = hook.Keystrokes
			}
			ev.DurationSec = hook.ActiveDurationSec
			if ev.DurationSec <= 0 {
				ev.DurationSec = 1
			}
		case "turn.started":
			ev.Type = "agent_start"
			ev.Category = tracker.CatAgentWorking
		case "turn.completed":
			ev.Type = "agent_stop"
			ev.Category = tracker.CatAgentWorking
			if hook.DurationMs > 0 {
				ev.DurationSec = hook.DurationMs / 1000
				if ev.DurationSec < 1 {
					ev.DurationSec = 1
				}
			}
		default:
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": hook.Type})
			return
		}

		hb := t.ProcessEvent(ev)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"heartbeat": hb,
			"status":    t.Status(),
		})
	})

	return mux
}

func addCORS(w http.ResponseWriter) {
	// Allows browser extension on Claude.ai / ChatGPT / v0 / Bolt to post to localhost daemon
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
