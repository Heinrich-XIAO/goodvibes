// SPDX-License-Identifier: AGPL-3.0-or-later
package server

import (
	"net/http"
	"os"

	"github.com/hackclub/goodvibes/internal/controllers"
	"github.com/hackclub/goodvibes/internal/convex"
	"github.com/hackclub/goodvibes/internal/realtime"
	"github.com/hackclub/goodvibes/internal/web"
)

// NewRouter builds the complete GoodVibes (Hackatime Go port) HTTP handler.
func NewRouter(cx *convex.Client) (http.Handler, *controllers.App) {
	app := controllers.NewApp(cx, realtime.NewHub())
	mux := http.NewServeMux()

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		dbOK := app.Convex.Query(r.Context(), "health:ping", nil, nil) == nil
		status := http.StatusOK
		if !dbOK {
			status = http.StatusServiceUnavailable
		}
		controllers.WriteJSON(w, status, map[string]any{
			"ok":              dbOK,
			"database":        "convex",
			"service":         "goodvibes-go",
			"auth_provider":   "hackclub",
			"auth_configured": os.Getenv("HACKCLUB_CLIENT_ID") != "" && os.Getenv("HACKCLUB_CLIENT_SECRET") != "",
		})
	}
	mux.HandleFunc("GET /up", healthHandler)
	mux.HandleFunc("GET /api/v1/health", healthHandler)

	// One-command daemon install (origin is baked into each script)
	mux.Handle("GET /install.sh", app.HandleInstaller("install.sh", "text/x-shellscript; charset=utf-8"))
	mux.Handle("GET /install.ps1", app.HandleInstaller("install.ps1", "text/plain; charset=utf-8"))

	// Hack Club Auth OAuth 2.0
	mux.HandleFunc("GET /api/v1/auth/hackclub", app.HandleHackClubAuthStart)
	mux.HandleFunc("GET /oauth/callback", app.HandleHackClubCallback)
	mux.HandleFunc("GET /api/v1/auth/hackclub/callback", app.HandleHackClubCallback)
	mux.HandleFunc("POST /api/v1/auth/logout", app.HandleLogout)
	mux.HandleFunc("DELETE /signout", app.HandleLogout)
	mux.HandleFunc("GET /api/v1/auth/me", app.HandleAuthMe)
	mux.HandleFunc("POST /api/v1/auth/api-keys", app.HandleCreateAPIKey)
	mux.HandleFunc("PATCH /api/v1/auth/settings", app.HandleUpdateSettings)

	// WakaTime / Hackatime-compatible heartbeat & statusbar endpoints
	mux.HandleFunc("POST /api/hackatime/v1/users/{id}/heartbeats", app.HandlePushHeartbeats)
	mux.HandleFunc("POST /api/hackatime/v1/users/{id}/heartbeats.bulk", app.HandlePushHeartbeats)
	mux.HandleFunc("GET /api/hackatime/v1/users/{id}/statusbar/today", app.HandleStatusBarToday)
	mux.HandleFunc("GET /api/hackatime/v1/users/current/stats/last_7_days", app.HandleStatsLast7Days)

	// GoodVibes native heartbeat ingestion
	mux.HandleFunc("POST /api/v1/heartbeats", app.HandlePushHeartbeats)

	// Stats, Spans, Trust Factor, Summary, Leaderboard, SSE Stream, SVG Badge
	mux.HandleFunc("GET /api/v1/users/{username}/stats", app.HandleUserStats)
	mux.HandleFunc("GET /api/v1/users/{username}/heartbeats/spans", app.HandleUserSpans)
	mux.HandleFunc("GET /api/v1/users/{username}/trust_factor", app.HandleTrustFactor)
	mux.HandleFunc("GET /api/v1/users/{username}/stream", app.HandleUserStream)
	mux.HandleFunc("GET /api/v1/badge/{username}", app.HandleBadgeSVG)
	mux.HandleFunc("GET /api/v1/leaderboard", app.HandleLeaderboard)
	mux.HandleFunc("GET /api/summary", app.HandleSummary)

	// Audit Bundles & Fraud Squad Admin API
	mux.HandleFunc("POST /api/v1/submissions", app.HandleCreateSubmission)
	mux.HandleFunc("GET /api/v1/submissions", app.HandleListSubmissions)
	mux.HandleFunc("GET /api/v1/submissions/{id}", app.HandleGetSubmission)
	mux.HandleFunc("POST /api/v1/submissions/{id}/review", app.HandleReviewSubmission)
	mux.HandleFunc("POST /api/admin/v1/user/convict", app.HandleAdminConvict)

	// Embedded real-data web app (no landing page)
	mux.Handle("/", web.Handler())
	return mux, app
}
