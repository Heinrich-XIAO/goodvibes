// SPDX-License-Identifier: AGPL-3.0-or-later
package syncer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hackclub/goodvibes/daemon/internal/config"
	"github.com/hackclub/goodvibes/daemon/internal/tracker"
)

// Client buffers heartbeats locally when offline and flushes them in batches
// to the centralized GoodVibes server.
type Client struct {
	mu        sync.Mutex
	cfg       config.Config
	queuePath string
	pending   []tracker.Heartbeat
	http      *http.Client
}

func New(cfg config.Config) *Client {
	c := &Client{
		cfg:       cfg,
		queuePath: filepath.Join(cfg.DataDir, "pending_heartbeats.json"),
		http:      &http.Client{Timeout: 10 * time.Second},
	}
	c.loadQueue()
	return c
}

func (c *Client) loadQueue() {
	data, err := os.ReadFile(c.queuePath)
	if err != nil {
		return
	}
	var items []tracker.Heartbeat
	if err := json.Unmarshal(data, &items); err == nil {
		c.pending = items
	}
}

func (c *Client) saveQueueLocked() {
	_ = os.MkdirAll(filepath.Dir(c.queuePath), 0o700)
	data, err := json.Marshal(c.pending)
	if err == nil {
		_ = os.WriteFile(c.queuePath, data, 0o600)
	}
}

// Enqueue adds a heartbeat and attempts an immediate flush if configured.
func (c *Client) Enqueue(hb tracker.Heartbeat) {
	c.mu.Lock()
	c.pending = append(c.pending, hb)
	c.saveQueueLocked()
	c.mu.Unlock()
	go func() {
		_ = c.Flush()
	}()
}

// Flush uploads all queued heartbeats to the central server.
func (c *Client) Flush() error {
	c.mu.Lock()
	if c.cfg.APIKey == "" {
		if latest, err := config.Load(); err == nil && latest.APIKey != "" {
			c.cfg = latest
		}
	}
	if len(c.pending) == 0 || c.cfg.APIKey == "" || c.cfg.ServerURL == "" {
		c.mu.Unlock()
		return nil
	}
	batch := make([]tracker.Heartbeat, len(c.pending))
	copy(batch, c.pending)
	c.mu.Unlock()

	payload, err := json.Marshal(map[string]any{"heartbeats": batch})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.cfg.ServerURL+"/api/v1/heartbeats", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}

	c.mu.Lock()
	if len(c.pending) >= len(batch) {
		c.pending = c.pending[len(batch):]
	} else {
		c.pending = nil
	}
	c.saveQueueLocked()
	c.mu.Unlock()
	return nil
}

// PendingCount returns how many heartbeats are queued offline.
func (c *Client) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}
