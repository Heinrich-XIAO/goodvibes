// SPDX-License-Identifier: AGPL-3.0-or-later

// Package vercelapp exposes the GoodVibes router to Vercel's Go runtime, which
// builds api/ as a separate module that cannot import internal/ packages.
package vercelapp

import (
	"net/http"
	"sync"

	"github.com/hackclub/goodvibes/internal/convex"
	"github.com/hackclub/goodvibes/internal/server"
)

var (
	once    sync.Once
	router  http.Handler
	initErr error
)

func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	once.Do(func() {
		var cx *convex.Client
		if cx, initErr = convex.FromEnv(); initErr == nil {
			router, _ = server.NewRouter(cx)
		}
	})
	if initErr != nil {
		http.Error(w, "init error: "+initErr.Error(), http.StatusInternalServerError)
		return
	}
	router.ServeHTTP(w, r)
}
