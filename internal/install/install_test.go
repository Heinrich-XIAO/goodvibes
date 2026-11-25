// SPDX-License-Identifier: AGPL-3.0-or-later
package install

import (
	"strings"
	"testing"
)

func TestRenderBakesInOrigin(t *testing.T) {
	for _, file := range []string{"install.sh", "install.ps1"} {
		body, err := Render(file, "https://goodvibes.example.com")
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", file, err)
		}
		script := string(body)
		if strings.Contains(script, serverPlaceholder) {
			t.Errorf("%s: placeholder left in rendered script", file)
		}
		if !strings.Contains(script, `"https://goodvibes.example.com"`) {
			t.Errorf("%s: origin not baked in", file)
		}
	}
}

func TestRenderRejectsOriginThatCouldBreakQuoting(t *testing.T) {
	for _, origin := range []string{
		`https://x.example"; rm -rf ~; "`,
		"https://x.example/path",
		"https://x.example$(id)",
		"ftp://x.example",
	} {
		if _, err := Render("install.sh", origin); err == nil {
			t.Errorf("origin %q was accepted", origin)
		}
	}
}
