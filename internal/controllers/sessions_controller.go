// SPDX-License-Identifier: AGPL-3.0-or-later
package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hackclub/goodvibes/internal/models"
)

type oauthStatePayload struct {
	Nonce string `json:"nonce"`
	Exp   int64  `json:"exp"`
}

type hackClubIdentity struct {
	ID                 string `json:"id"`
	FirstName          string `json:"first_name"`
	LastName           string `json:"last_name"`
	PrimaryEmail       string `json:"primary_email"`
	SlackID            string `json:"slack_id"`
	VerificationStatus string `json:"verification_status"`
	YSWSEligible       bool   `json:"ysws_eligible"`
}

var nonSlug = regexp.MustCompile(`[^a-z0-9_-]`)

func (a *App) redirectURI(r *http.Request) string {
	if env := strings.TrimSpace(os.Getenv("HACKCLUB_REDIRECT_URI")); env != "" {
		return env
	}
	return a.Origin(r) + "/oauth/callback"
}

// HandleHackClubAuthStart handles GET /api/v1/auth/hackclub.
func (a *App) HandleHackClubAuthStart(w http.ResponseWriter, r *http.Request) {
	clientID := os.Getenv("HACKCLUB_CLIENT_ID")
	clientSecret := os.Getenv("HACKCLUB_CLIENT_SECRET")
	redir := a.redirectURI(r)
	if clientID == "" || clientSecret == "" {
		WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "Hack Club Auth is not configured yet.",
			"setup": map[string]any{
				"create_app_url":    "https://auth.hackclub.com/developer/apps/new",
				"redirect_uri":      redir,
				"required_env_vars": []string{"HACKCLUB_CLIENT_ID", "HACKCLUB_CLIENT_SECRET"},
			},
		})
		return
	}

	state := a.SignToken(oauthStatePayload{
		Nonce: RandomHex(16),
		Exp:   time.Now().Add(10 * time.Minute).Unix(),
	})

	u, _ := url.Parse(HackClubAuthBase + "/oauth/authorize")
	q := u.Query()
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redir)
	q.Set("response_type", "code")
	q.Set("scope", HackClubScopes)
	q.Set("state", state)
	u.RawQuery = q.Encode()

	http.SetCookie(w, &http.Cookie{
		Name:     "gv_oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.HasPrefix(a.Origin(r), "https://"),
		MaxAge:   600,
	})
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// HandleHackClubCallback handles GET /oauth/callback (and /api/v1/auth/hackclub/callback).
func (a *App) HandleHackClubCallback(w http.ResponseWriter, r *http.Request) {
	if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
		http.Redirect(w, r, "/?auth_error="+url.QueryEscape(oauthErr), http.StatusFound)
		return
	}
	code := r.URL.Query().Get("code")
	stateParam := r.URL.Query().Get("state")
	if code == "" || stateParam == "" {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Missing OAuth code or state"})
		return
	}

	stateCookie, err := r.Cookie("gv_oauth_state")
	var payload oauthStatePayload
	if err != nil || stateCookie.Value != stateParam || !a.VerifySignedToken(stateParam, &payload) || payload.Exp < time.Now().Unix() {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid or expired OAuth state"})
		return
	}

	// 1. Exchange authorization code at POST https://auth.hackclub.com/oauth/token
	tokenReqBody, _ := json.Marshal(map[string]string{
		"client_id":     os.Getenv("HACKCLUB_CLIENT_ID"),
		"client_secret": os.Getenv("HACKCLUB_CLIENT_SECRET"),
		"redirect_uri":  a.redirectURI(r),
		"code":          code,
		"grant_type":    "authorization_code",
	})
	tokenResp, err := a.HTTPClient.Post(HackClubAuthBase+"/oauth/token", "application/json", bytes.NewReader(tokenReqBody))
	if err != nil {
		WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "Token exchange failed: " + err.Error()})
		return
	}
	defer tokenResp.Body.Close()

	var tokenData struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	_ = json.NewDecoder(tokenResp.Body).Decode(&tokenData)
	if tokenResp.StatusCode < 200 || tokenResp.StatusCode >= 300 || tokenData.AccessToken == "" {
		WriteJSON(w, http.StatusBadGateway, map[string]string{
			"error": "Hack Club Auth token exchange failed: " + tokenData.ErrorDesc + tokenData.Error,
		})
		return
	}

	// 2. Fetch identity at GET https://auth.hackclub.com/api/v1/me
	meReq, _ := http.NewRequest("GET", HackClubAuthBase+"/api/v1/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	meResp, err := a.HTTPClient.Do(meReq)
	if err != nil {
		WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "Failed to fetch /api/v1/me: " + err.Error()})
		return
	}
	defer meResp.Body.Close()

	var meData struct {
		Identity hackClubIdentity `json:"identity"`
	}
	_ = json.NewDecoder(meResp.Body).Decode(&meData)
	if meResp.StatusCode < 200 || meResp.StatusCode >= 300 || meData.Identity.ID == "" {
		WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "Invalid identity response from Hack Club Auth"})
		return
	}

	user, err := a.upsertHackClubUser(r.Context(), meData.Identity)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	sessToken, err := a.CreateSession(r.Context(), user.ID)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "create session failed"})
		return
	}

	isHTTPS := strings.HasPrefix(a.Origin(r), "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     "gv_session",
		Value:    sessToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS,
		MaxAge:   int(SessionTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "gv_oauth_state",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/u/"+url.PathEscape(user.Username), http.StatusFound)
}

func (a *App) upsertHackClubUser(ctx context.Context, ident hackClubIdentity) (*models.User, error) {
	email := strings.ToLower(strings.TrimSpace(ident.PrimaryEmail))
	displayName := strings.TrimSpace(ident.FirstName + " " + ident.LastName)
	if displayName == "" && email != "" {
		displayName = strings.Split(email, "@")[0]
	}
	if displayName == "" {
		displayName = ident.ID
	}

	adminLevel := 0
	for _, e := range strings.Split(os.Getenv("REVIEWER_EMAILS"), ",") {
		if strings.ToLower(strings.TrimSpace(e)) == email && email != "" {
			adminLevel = int(models.AdminLevelSuperadmin)
			break
		}
	}

	base := displayName
	if email != "" {
		base = strings.Split(email, "@")[0]
	}
	base = nonSlug.ReplaceAllString(strings.ToLower(base), "")
	if len(base) > 20 {
		base = base[:20]
	}
	if len(base) < 3 {
		base = "hc_" + nonSlug.ReplaceAllString(strings.ToLower(ident.ID), "")
		if len(base) > 12 {
			base = base[:12]
		}
	}

	var u *models.User
	err := a.Convex.Mutation(ctx, "users:upsertHackClub", map[string]any{
		"hackclub_id":         ident.ID,
		"email":               email,
		"display_name":        displayName,
		"slack_uid":           ident.SlackID,
		"verification_status": ident.VerificationStatus,
		"ysws_eligible":       ident.YSWSEligible,
		"admin_level":         adminLevel,
		"username_base":       base,
		"api_key":             GenerateAPIKeyToken(),
	}, &u)
	if err == nil && u == nil {
		err = fmt.Errorf("upsert returned no user")
	}
	return u, err
}

func (a *App) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("gv_session"); err == nil && c.Value != "" {
		_ = a.Convex.Mutation(r.Context(), "users:deleteSession", map[string]any{"token": c.Value}, nil)
	}
	http.SetCookie(w, &http.Cookie{Name: "gv_session", Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) HandleAuthMe(w http.ResponseWriter, r *http.Request) {
	configured := os.Getenv("HACKCLUB_CLIENT_ID") != "" && os.Getenv("HACKCLUB_CLIENT_SECRET") != ""
	u := a.CurrentUser(r)
	if u == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"user": nil, "auth_configured": configured})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"user": u, "auth_configured": configured})
}

func (a *App) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	u := a.CurrentUser(r)
	if u == nil {
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	_ = ReadJSON(r, &body, 1<<16)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = fmt.Sprintf("key-%d", time.Now().Unix())
	}
	tok := GenerateAPIKeyToken()
	err := a.Convex.Mutation(r.Context(), "users:createApiKey", map[string]any{"user_id": u.ID, "name": name, "token": tok}, nil)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]string{"api_key": tok, "name": name})
}

func (a *App) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	u := a.CurrentUser(r)
	if u == nil {
		WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
		return
	}
	var body struct {
		AllowPublicStatsLookup *bool   `json:"allow_public_stats_lookup"`
		Bio                    *string `json:"bio"`
		Timezone               *string `json:"timezone"`
	}
	if err := ReadJSON(r, &body, 1<<16); err != nil {
		WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	patch := map[string]any{"user_id": u.ID}
	if body.AllowPublicStatsLookup != nil {
		patch["allow_public_stats_lookup"] = *body.AllowPublicStatsLookup
	}
	if body.Bio != nil {
		patch["bio"] = *body.Bio
	}
	if body.Timezone != nil && *body.Timezone != "" {
		patch["timezone"] = *body.Timezone
	}
	var updated *models.User
	if err := a.Convex.Mutation(r.Context(), "users:updateSettings", patch, &updated); err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"user": updated})
}
