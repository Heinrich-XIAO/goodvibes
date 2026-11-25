// SPDX-License-Identifier: AGPL-3.0-or-later
package controllers

import (
	"bytes"
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

	user, err := a.upsertHackClubUser(meData.Identity)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	sessToken, err := a.CreateSession(user.ID)
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

func (a *App) upsertHackClubUser(ident hackClubIdentity) (*models.User, error) {
	email := strings.ToLower(strings.TrimSpace(ident.PrimaryEmail))
	displayName := strings.TrimSpace(ident.FirstName + " " + ident.LastName)
	if displayName == "" && email != "" {
		displayName = strings.Split(email, "@")[0]
	}
	if displayName == "" {
		displayName = ident.ID
	}

	yswsInt := 0
	if ident.YSWSEligible {
		yswsInt = 1
	}

	adminLevel := 0
	for _, e := range strings.Split(os.Getenv("REVIEWER_EMAILS"), ",") {
		if strings.ToLower(strings.TrimSpace(e)) == email && email != "" {
			adminLevel = int(models.AdminLevelSuperadmin)
			break
		}
	}

	var existingID int64
	err := a.DB.QueryRow("SELECT id FROM users WHERE hackclub_id = ?", ident.ID).Scan(&existingID)
	if err == nil {
		_, err = a.DB.Exec(`
			UPDATE users
			SET display_name = ?, slack_uid = CASE WHEN ? != '' THEN ? ELSE slack_uid END,
			    verification_status = ?, ysws_eligible = ?, admin_level = MAX(admin_level, ?),
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = ?`,
			displayName, ident.SlackID, ident.SlackID, ident.VerificationStatus, yswsInt, adminLevel, existingID,
		)
		return a.LoadUserByID(existingID), err
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

	username := base
	for i := 1; ; i++ {
		var tmp int64
		if err := a.DB.QueryRow("SELECT id FROM users WHERE LOWER(username) = LOWER(?)", username).Scan(&tmp); err != nil {
			break
		}
		username = fmt.Sprintf("%s%d", base, i)
	}

	var slackPtr any = nil
	if ident.SlackID != "" {
		slackPtr = ident.SlackID
	}
	res, err := a.DB.Exec(`
		INSERT INTO users (hackclub_id, slack_uid, username, display_name, verification_status, ysws_eligible, allow_public_stats_lookup, admin_level)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?)`,
		ident.ID, slackPtr, username, displayName, ident.VerificationStatus, yswsInt, adminLevel,
	)
	if err != nil {
		return nil, err
	}
	uid, _ := res.LastInsertId()
	if email != "" {
		_, _ = a.DB.Exec("INSERT OR IGNORE INTO email_addresses (email, user_id, source) VALUES (?, ?, 0)", email, uid)
	}
	// Mint default WakaTime / GoodVibes API key
	_, _ = a.DB.Exec("INSERT INTO api_keys (user_id, name, token) VALUES (?, 'default', ?)", uid, GenerateAPIKeyToken())
	return a.LoadUserByID(uid), nil
}

func (a *App) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("gv_session"); err == nil && c.Value != "" {
		_, _ = a.DB.Exec("DELETE FROM sessions WHERE token = ?", c.Value)
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
	_, err := a.DB.Exec("INSERT INTO api_keys (user_id, name, token) VALUES (?, ?, ?)", u.ID, name, tok)
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
	if body.AllowPublicStatsLookup != nil {
		v := 0
		if *body.AllowPublicStatsLookup {
			v = 1
		}
		_, _ = a.DB.Exec("UPDATE users SET allow_public_stats_lookup = ? WHERE id = ?", v, u.ID)
	}
	if body.Bio != nil {
		_, _ = a.DB.Exec("UPDATE users SET bio = ? WHERE id = ?", *body.Bio, u.ID)
	}
	if body.Timezone != nil && *body.Timezone != "" {
		_, _ = a.DB.Exec("UPDATE users SET timezone = ? WHERE id = ?", *body.Timezone, u.ID)
	}
	WriteJSON(w, http.StatusOK, map[string]any{"user": a.LoadUserByID(u.ID)})
}
