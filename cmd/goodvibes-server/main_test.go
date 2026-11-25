// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/hackclub/goodvibes/internal/controllers"
	"github.com/hackclub/goodvibes/internal/convex"
	"github.com/hackclub/goodvibes/internal/server"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEndToEndGoHackatimePort(t *testing.T) {
	t.Setenv("HACKCLUB_CLIENT_ID", "test_cid")
	t.Setenv("HACKCLUB_CLIENT_SECRET", "test_csec")
	t.Setenv("HACKCLUB_REDIRECT_URI", "http://localhost:3000/oauth/callback")

	if os.Getenv("GOODVIBES_E2E") != "1" {
		t.Skip("set GOODVIBES_E2E=1 to run against the Convex deployment in .env.local (it writes real rows)")
	}
	loadDotEnv("../../.env.local")
	cx, err := convex.FromEnv()
	if err != nil {
		t.Skipf("needs a Convex dev deployment: %v", err)
	}
	run := controllers.RandomHex(4)
	username := "rowan" + run

	router, app := server.NewRouter(cx)
	app.HTTPClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/oauth/token":
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"access_token":"idntk.go_test"}`)),
				}, nil
			case "/api/v1/me":
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{
						"identity": {
							"id": "ident!go` + run + `",
							"first_name": "Rowan",
							"last_name": "Go",
							"primary_email": "` + username + `@hackclub.com",
							"slack_id": "U` + run + `",
							"verification_status": "verified",
							"ysws_eligible": true
						}
					}`)),
				}, nil
			default:
				t.Fatalf("unexpected external request: %s", req.URL.String())
				return nil, nil
			}
		}),
	}

	ts := httptest.NewServer(router)
	defer ts.Close()

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	// 1. Start Hack Club Auth OAuth flow
	res1, err := noRedirect.Get(ts.URL + "/api/v1/auth/hackclub")
	if err != nil || res1.StatusCode != 302 {
		t.Fatalf("expected 302 from /api/v1/auth/hackclub, got %v %v", res1, err)
	}
	loc, _ := url.Parse(res1.Header.Get("Location"))
	stateParam := loc.Query().Get("state")

	// 2. Complete OAuth callback at /oauth/callback
	cbReq, _ := http.NewRequest("GET", ts.URL+"/oauth/callback?code=xyz&state="+url.QueryEscape(stateParam), nil)
	for _, c := range res1.Cookies() {
		cbReq.AddCookie(c)
	}
	res2, err := noRedirect.Do(cbReq)
	if err != nil || res2.StatusCode != 302 || res2.Header.Get("Location") != "/u/"+username {
		body, _ := io.ReadAll(res2.Body)
		t.Fatalf("expected redirect to /u/%s, got status=%d loc=%q err=%v body=%s", username, res2.StatusCode, res2.Header.Get("Location"), err, body)
	}
	var sessionCookie *http.Cookie
	for _, c := range res2.Cookies() {
		if c.Name == "gv_session" {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatalf("expected gv_session cookie")
	}

	// 3. Mint API key
	keyReq, _ := http.NewRequest("POST", ts.URL+"/api/v1/auth/api-keys", strings.NewReader(`{"name":"laptop"}`))
	keyReq.AddCookie(sessionCookie)
	res3, err := http.DefaultClient.Do(keyReq)
	if err != nil || res3.StatusCode != 201 {
		t.Fatalf("expected 201 from api-keys, got %v %v", res3, err)
	}
	var keyResp struct {
		APIKey string `json:"api_key"`
	}
	_ = json.NewDecoder(res3.Body).Decode(&keyResp)

	// 4. Push heartbeats (60s prompt_typing, 40s prompt_vtt, 300s agent_working which MUST be excluded!)
	hbBody := `{
		"heartbeats": [
			{"project":"cs2-browser","category":"prompt_typing","human_seconds":60,"harness":"t3-code","struggle_score":88,"evidence_hash":"aaaa1111bbbb2222cccc3333dddd` + run + `"},
			{"project":"cs2-browser","category":"prompt_vtt","human_seconds":40,"harness":"whisperflow","struggle_score":86,"evidence_hash":"eeee5555ffff6666aaaa7777bbbb8888"},
			{"project":"cs2-browser","category":"agent_working","human_seconds":300,"agent_seconds":300,"harness":"t3-code","evidence_hash":"cccc9999dddd0000eeee1111ffff2222"}
		]
	}`
	hbReq, _ := http.NewRequest("POST", ts.URL+"/api/v1/heartbeats", bytes.NewBufferString(hbBody))
	hbReq.Header.Set("Authorization", "Bearer "+keyResp.APIKey)
	res4, err := http.DefaultClient.Do(hbReq)
	if err != nil || res4.StatusCode != 202 {
		t.Fatalf("expected 202 from heartbeats, got %v %v", res4, err)
	}

	// 5. Verify stats: 100s human, 300s excluded agent
	res5, err := http.Get(ts.URL + "/api/v1/users/" + username + "/stats")
	if err != nil || res5.StatusCode != 200 {
		t.Fatalf("expected 200 from stats, got %v %v", res5, err)
	}
	var statsResp struct {
		Stats struct {
			Totals struct {
				HumanSeconds int `json:"human_seconds"`
				AgentSeconds int `json:"agent_seconds"`
			} `json:"totals"`
		} `json:"stats"`
	}
	_ = json.NewDecoder(res5.Body).Decode(&statsResp)
	if statsResp.Stats.Totals.HumanSeconds != 100 || statsResp.Stats.Totals.AgentSeconds != 300 {
		t.Fatalf("expected 100s human / 300s agent, got %+v", statsResp.Stats.Totals)
	}

	// 6. Verify Audit Bundle cross-check
	subBody := `{
		"project": "cs2-browser",
		"program": "Stardance",
		"bundle": {
			"chain_valid": true,
			"entries": [
				{"seq":1,"category":"prompt_typing","duration_sec":60,"struggle_score":88,"hash":"aaaa1111bbbb2222cccc3333dddd` + run + `"},
				{"seq":2,"category":"prompt_vtt","duration_sec":40,"struggle_score":86,"hash":"eeee5555ffff6666aaaa7777bbbb8888"},
				{"seq":3,"category":"agent_working","duration_sec":300,"agent_sec":300,"struggle_score":0,"hash":"cccc9999dddd0000eeee1111ffff2222"}
			]
		}
	}`
	subReq, _ := http.NewRequest("POST", ts.URL+"/api/v1/submissions", bytes.NewBufferString(subBody))
	subReq.Header.Set("Authorization", "Bearer "+keyResp.APIKey)
	res6, err := http.DefaultClient.Do(subReq)
	if err != nil || res6.StatusCode != 201 {
		t.Fatalf("expected 201 from submissions, got %v %v", res6, err)
	}
	var subResp struct {
		Verification struct {
			HeartbeatMatchRate   int `json:"heartbeat_match_rate"`
			ClaimedHumanSeconds  int `json:"claimed_human_seconds"`
			ExcludedAgentSeconds int `json:"excluded_agent_seconds"`
			TrustScore           int `json:"trust_score"`
		} `json:"verification"`
	}
	_ = json.NewDecoder(res6.Body).Decode(&subResp)
	if subResp.Verification.HeartbeatMatchRate != 100 || subResp.Verification.ClaimedHumanSeconds != 100 || subResp.Verification.ExcludedAgentSeconds != 300 {
		t.Fatalf("unexpected verification report: %+v", subResp.Verification)
	}
}
