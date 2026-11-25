// SPDX-License-Identifier: AGPL-3.0-or-later
package controllers

import (
	"net/http"

	"github.com/hackclub/goodvibes/internal/install"
)

// HandleInstaller serves an installer script with this server's origin filled in.
func (a *App) HandleInstaller(file, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := install.Render(file, a.Origin(r))
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}
}
