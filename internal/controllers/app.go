// SPDX-License-Identifier: AGPL-3.0-or-later
package controllers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hackclub/goodvibes/internal/convex"
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
	Convex     *convex.Client
	Hub        *realtime.Hub
	HTTPClient *http.Client
}

func NewApp(cx *convex.Client, hub *realtime.Hub) *App {
	return &App{
		Convex:     cx,
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
	session := ""
	if c, err := r.Cookie("gv_session"); err == nil {
		session = c.Value
	}
	if token == "" && session == "" {
		return nil
	}
	var u *models.User
	err := a.Convex.Query(r.Context(), "users:current", map[string]any{
		"api_key": token, "session_token": session, "now": time.Now().Unix(),
	}, &u)
	if err != nil {
		return nil
	}
	return u
}

// LoadUser finds a user by username (case-insensitive), Slack ID, or Convex ID.
func (a *App) LoadUser(ctx context.Context, ident string) *models.User {
	var u *models.User
	if err := a.Convex.Query(ctx, "users:get", map[string]any{"ident": ident}, &u); err != nil {
		return nil
	}
	return u
}

func (a *App) CreateSession(ctx context.Context, userID string) (string, error) {
	tok := RandomHex(32)
	err := a.Convex.Mutation(ctx, "users:createSession", map[string]any{
		"user_id": userID, "token": tok, "expires_at": time.Now().Add(SessionTTL).Unix(),
	}, nil)
	return tok, err
}
