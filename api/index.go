// SPDX-License-Identifier: AGPL-3.0-or-later
package handler

import (
	"net/http"

	"github.com/hackclub/goodvibes/pkg/vercelapp"
)

// Handler is the Vercel Go serverless entrypoint for GoodVibes.
func Handler(w http.ResponseWriter, r *http.Request) {
	vercelapp.ServeHTTP(w, r)
}
