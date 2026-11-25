// SPDX-License-Identifier: AGPL-3.0-or-later
package controllers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hackclub/goodvibes/internal/models"
	"github.com/hackclub/goodvibes/internal/realtime"
)

const (
	HackClubAuthBase = "https://auth.hackclub.com"
	HackClubScopes   = "openid profile email name slack_id verification_status"
	SessionTTL       = 30 * 24 * time.Hour
)

// App holds shared dependencies across all ported Hackatime/GoodVibes controllers.
type App struct {
	DB         *sql.DB
	Hub        *realtime.Hub
	HTTPClient *http.Client
}

func NewApp(conn *sql.DB, hub *realtime.Hub) *App {
	return &App{
		DB:         conn,
		Hub:        hub,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *App) signingSecret() []byte {
	if s := os.Getenv("SESSION_SECRET"); s != "" {
		return []byte(s)
	}
	if s := os.Getenv("HACKCLUB_CLIENT_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte("goodvibes-local-dev-secret")
}

func (a *App) SignToken(payload any) string {
	raw, _ := json.Marshal(payload)
	b64 := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, a.signingSecret())
	_, _ = mac.Write([]byte(b64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return b64 + "." + sig
}

func (a *App) VerifySignedToken(tok string, out any) bool {
	parts := strings.Split(tok, ".")
	if len(parts) != 2 {
		return false
	}
	mac := hmac.New(sha256.New, a.signingSecret())
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[1]), []byte(expected)) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

func RandomHex(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func GenerateAPIKeyToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "gv_live_" + base64.RawURLEncoding.EncodeToString(b)
}

func (a *App) Origin(r *http.Request) string {
	if pub := strings.TrimRight(os.Getenv("PUBLIC_URL"), "/"); pub != "" {
		return pub
	}
	proto := "http"
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		proto = strings.TrimSpace(strings.Split(p, ",")[0])
	} else if r.TLS != nil {
		proto = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	if host == "" {
		host = "localhost:3000"
	}
	return proto + "://" + host
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ReadJSON(r *http.Request, out any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBytes))
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

// CurrentUser resolves the authenticated user via Bearer/Basic API key,
// `api_key` query param (WakaTime compatibility), or `gv_session` cookie.
func (a *App) CurrentUser(r *http.Request) *models.User {
	// 1. Bearer or Basic Authorization header (WakaTime sends `Basic base64(api_key)`)
	auth := r.Header.Get("Authorization")
	var token string
	if strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	} else if strings.HasPrefix(auth, "Basic ") {
		if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(auth, "Basic "))); err == nil {
			token = strings.TrimSuffix(string(decoded), ":")
		}
	}
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("api_key"))
	}
	if token != "" {
		var uid int64
		err := a.DB.QueryRow("SELECT user_id FROM api_keys WHERE token = ? LIMIT 1", token).Scan(&uid)
		if err == nil {
			return a.LoadUserByID(uid)
		}
	}

	// 2. Browser session cookie
	cookie, err := r.Cookie("gv_session")
	if err != nil || cookie.Value == "" {
		return nil
	}
	var uid int64
	now := time.Now().Unix()
	err = a.DB.QueryRow("SELECT user_id FROM sessions WHERE token = ? AND expires_at > ?", cookie.Value, now).Scan(&uid)
	if err != nil {
		return nil
	}
	return a.LoadUserByID(uid)
}

func (a *App) LoadUserByID(id int64) *models.User {
	return a.scanUser(a.DB.QueryRow(`
		SELECT id, COALESCE(hackclub_id,''), COALESCE(username,''), COALESCE(display_name,''),
		       COALESCE(slack_uid,''), COALESCE(github_uid,''), COALESCE(github_username,''),
		       COALESCE(timezone,'UTC'), COALESCE(country_code,''), COALESCE(verification_status,''),
		       ysws_eligible, COALESCE(bio,''), allow_public_stats_lookup, default_timezone_leaderboard,
		       trust_level, admin_level, hackatime_extension_text_type
		FROM users WHERE id = ?`, id))
}

func (a *App) LoadUserByUsernameOrID(ident string) *models.User {
	return a.scanUser(a.DB.QueryRow(`
		SELECT id, COALESCE(hackclub_id,''), COALESCE(username,''), COALESCE(display_name,''),
		       COALESCE(slack_uid,''), COALESCE(github_uid,''), COALESCE(github_username,''),
		       COALESCE(timezone,'UTC'), COALESCE(country_code,''), COALESCE(verification_status,''),
		       ysws_eligible, COALESCE(bio,''), allow_public_stats_lookup, default_timezone_leaderboard,
		       trust_level, admin_level, hackatime_extension_text_type
		FROM users WHERE LOWER(username) = LOWER(?) OR slack_uid = ? OR CAST(id AS TEXT) = ? LIMIT 1`,
		ident, ident, ident))
}

func (a *App) scanUser(row *sql.Row) *models.User {
	var u models.User
	var ysws, pub, tzLead int
	var tl, al, ext int
	err := row.Scan(
		&u.ID, &u.HackClubID, &u.Username, &u.DisplayName,
		&u.SlackUID, &u.GithubUID, &u.GithubUsername,
		&u.Timezone, &u.CountryCode, &u.VerificationStatus,
		&ysws, &u.Bio, &pub, &tzLead, &tl, &al, &ext,
	)
	if err != nil {
		return nil
	}
	u.YSWSEligible = ysws != 0
	u.AllowPublicStatsLookup = pub != 0
	u.DefaultTimezoneLeaderboard = tzLead != 0
	u.TrustLevel = models.TrustLevel(tl)
	u.AdminLevel = models.AdminLevel(al)
	u.HackatimeExtensionTextType = models.HackatimeExtensionTextType(ext)
	return &u
}

func (a *App) CreateSession(userID int64) (string, error) {
	tok := RandomHex(32)
	now := time.Now().Unix()
	_, err := a.DB.Exec(
		"INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		tok, userID, now, now+int64(SessionTTL.Seconds()),
	)
	return tok, err
}

func ErrNotFound() error { return errors.New("not found") }
