// SPDX-License-Identifier: AGPL-3.0-or-later
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed assets/*
var assetsFS embed.FS

// Handler serves the embedded GoodVibes real-data web app (no landing page).
func Handler() http.Handler {
	sub, _ := fs.Sub(assetsFS, "assets")
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(r.URL.Path)
		if p == "/" || strings.HasPrefix(p, "/u/") || p == "/audit" || p == "/setup" || p == "/leaderboards" {
			data, err := assetsFS.ReadFile("assets/index.html")
			if err != nil {
				http.Error(w, "missing index.html", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
