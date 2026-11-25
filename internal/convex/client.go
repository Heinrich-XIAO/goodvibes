// SPDX-License-Identifier: AGPL-3.0-or-later

// Package convex calls GoodVibes' Convex functions over the Convex HTTP API.
package convex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	url    string
	secret string
	http   *http.Client
}

// FromEnv builds a client from CONVEX_URL and GOODVIBES_SERVER_SECRET.
func FromEnv() (*Client, error) {
	url := strings.TrimRight(os.Getenv("CONVEX_URL"), "/")
	secret := os.Getenv("GOODVIBES_SERVER_SECRET")
	if url == "" || secret == "" {
		return nil, errors.New("CONVEX_URL and GOODVIBES_SERVER_SECRET must be set")
	}
	return &Client{url: url, secret: secret, http: &http.Client{Timeout: 15 * time.Second}}, nil
}

// Query runs a Convex query such as "users:get" and decodes its value into out.
func (c *Client) Query(ctx context.Context, path string, args map[string]any, out any) error {
	return c.call(ctx, "query", path, args, out)
}

// Mutation runs a Convex mutation and decodes its value into out (which may be nil).
func (c *Client) Mutation(ctx context.Context, path string, args map[string]any, out any) error {
	return c.call(ctx, "mutation", path, args, out)
}

func (c *Client) call(ctx context.Context, kind, path string, args map[string]any, out any) error {
	withSecret := make(map[string]any, len(args)+1)
	for k, v := range args {
		withSecret[k] = v
	}
	withSecret["secret"] = c.secret
	body, err := json.Marshal(map[string]any{"path": path, "args": withSecret, "format": "json"})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/api/"+kind, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("convex %s: %w", path, err)
	}
	defer res.Body.Close()
	var env struct {
		Status       string          `json:"status"`
		Value        json.RawMessage `json:"value"`
		ErrorMessage string          `json:"errorMessage"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		return fmt.Errorf("convex %s: http %d: %w", path, res.StatusCode, err)
	}
	if env.Status != "success" {
		return fmt.Errorf("convex %s: %s", path, env.ErrorMessage)
	}
	if out == nil {
		return nil
	}
	return decodeValue(env.Value, out)
}

// decodeValue unmarshals a Convex value into out. Convex sends every number as
// a float ("3.0"), which encoding/json rejects for Go integer fields, so
// integral floats are rewritten as integers first.
func decodeValue(raw json.RawMessage, out any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	fixed, err := json.Marshal(integralNumbers(v))
	if err != nil {
		return err
	}
	return json.Unmarshal(fixed, out)
}

func integralNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, e := range t {
			t[k] = integralNumbers(e)
		}
	case []any:
		for i, e := range t {
			t[i] = integralNumbers(e)
		}
	case json.Number:
		if f, err := t.Float64(); err == nil && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
			return json.Number(strconv.FormatInt(int64(f), 10))
		}
	}
	return v
}
