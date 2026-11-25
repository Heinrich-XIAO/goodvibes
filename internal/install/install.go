// SPDX-License-Identifier: AGPL-3.0-or-later
package install

import (
	"bytes"
	"embed"
	"fmt"
	"regexp"
)

//go:embed install.sh install.ps1
var scripts embed.FS

const serverPlaceholder = "__GOODVIBES_SERVER__"

// The origin is pasted inside quotes in both scripts, so only a plain scheme://host[:port] passes.
var safeOrigin = regexp.MustCompile(`^https?://[A-Za-z0-9.\-\[\]:]+$`)

// Render returns the installer file (install.sh or install.ps1) with the server origin filled in.
func Render(file, origin string) ([]byte, error) {
	if !safeOrigin.MatchString(origin) {
		return nil, fmt.Errorf("refusing to install from unsafe origin %q", origin)
	}
	data, err := scripts.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(data, []byte(serverPlaceholder), []byte(origin)), nil
}
