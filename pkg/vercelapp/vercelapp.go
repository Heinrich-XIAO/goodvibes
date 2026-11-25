// SPDX-License-Identifier: AGPL-3.0-or-later

// Package vercelapp exposes the GoodVibes HTTP handler to the Vercel Go
// runtime, which builds api/ outside this module and so cannot import
// internal/ packages directly.
package vercelapp

import (
	"net/http"
	"os"
	"sync"

	"github.com/hackclub/goodvibes/internal/db"
	"github.com/hackclub/goodvibes/internal/server"
)

var (
	once    sync.Once
	router  http.Handler
	initErr error
)

func initRouter() {
	dbPath := os.Getenv("GOODVIBES_DB")
	if dbPath == "" {
		dbPath = "/tmp/goodvibes.db"
	}
	conn, err := db.Open(dbPath)
	if err != nil {
		initErr = err
		return
	}
	router, _ = server.NewRouter(conn)
}

// ServeHTTP lazily opens the database and serves the GoodVibes router.
func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	once.Do(initRouter)
	if initErr != nil {
		http.Error(w, "init error: "+initErr.Error(), http.StatusInternalServerError)
		return
	}
	router.ServeHTTP(w, r)
}
