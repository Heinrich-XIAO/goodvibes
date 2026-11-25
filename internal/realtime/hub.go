// SPDX-License-Identifier: AGPL-3.0-or-later
package realtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Hub manages Server-Sent Events (SSE) subscribers keyed by lowercase username.
type Hub struct {
	mu       sync.RWMutex
	channels map[string]map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{
		channels: make(map[string]map[chan []byte]struct{}),
	}
}

func (h *Hub) Publish(username, eventName string, payload any) {
	key := strings.ToLower(strings.TrimSpace(username))
	if key == "" {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	msg := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventName, data))

	h.mu.RLock()
	subs := h.channels[key]
	for ch := range subs {
		select {
		case ch <- msg:
		default:
		}
	}
	h.mu.RUnlock()
}

func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request, username string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	key := strings.ToLower(strings.TrimSpace(username))
	ch := make(chan []byte, 16)

	h.mu.Lock()
	if h.channels[key] == nil {
		h.channels[key] = make(map[chan []byte]struct{})
	}
	h.channels[key][ch] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.channels[key], ch)
		if len(h.channels[key]) == 0 {
			delete(h.channels, key)
		}
		h.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"username\":%q,\"ts\":%d}\n\n", key, time.Now().Unix())
	flusher.Flush()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			_, _ = w.Write(msg)
			flusher.Flush()
		case <-ping.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}
