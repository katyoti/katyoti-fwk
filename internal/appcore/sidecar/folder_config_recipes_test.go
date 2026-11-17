// SPDX-License-Identifier: Apache-2.0

package sidecar

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// A files-only sidecar declares its programs in its ktw-host.yaml `config:`
// block as a compiled one does: required or optional, with a recipe per
// platform. The in-process config check carries them; a recipe an installer
// engine could not run is refused.
func TestFolderConfig_ToolRecipes(t *testing.T) {
	const block = `
tools:
  - name: rclone
    commands: [ktw-fixture-rclone-absent]
    installSkill: ktw-fixture:/skill/install-rclone
    recipes:
      - {platform: windows, kind: winget, package: Rclone.Rclone, minVersion: "1.68"}
      - {platform: darwin, kind: brew, package: rclone}
      - {platform: linux, kind: script, script: scripts/install-rclone.sh}
  - name: ffmpeg
    commands: [ktw-fixture-ffmpeg-absent]
    optional: true
    recipes:
      - {platform: linux, kind: apt, package: ffmpeg}
`
	var c FolderConfig
	if err := config.DecodeBytes([]byte(block), marshal.YAML, &c); err != nil {
		t.Fatal(err)
	}
	report, err := CheckFolderConfig(context.Background(), t.TempDir(), "ktw-fixture", c)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Deps) != 2 {
		t.Fatalf("deps = %+v", report.Deps)
	}
	rclone, ffmpeg := report.Deps[0], report.Deps[1]
	if !rclone.Required || ffmpeg.Required {
		t.Errorf("required: rclone %v ffmpeg %v, want true / false", rclone.Required, ffmpeg.Required)
	}
	want := []publish.InstallRecipe{
		{Platform: "windows", Kind: "winget", Package: "Rclone.Rclone", MinVersion: "1.68"},
		{Platform: "darwin", Kind: "brew", Package: "rclone"},
		{Platform: "linux", Kind: "script", Script: "scripts/install-rclone.sh"},
	}
	if len(rclone.Recipes) != len(want) {
		t.Fatalf("rclone recipes = %+v", rclone.Recipes)
	}
	for i := range want {
		if rclone.Recipes[i] != want[i] {
			t.Errorf("rclone recipe %d = %+v, want %+v", i, rclone.Recipes[i], want[i])
		}
	}
	for _, it := range report.Items {
		if it.Category == "program" && it.Name == "ffmpeg" && it.Severity != "warn" {
			t.Errorf("optional ffmpeg missing: severity %q, want warn", it.Severity)
		}
		if it.Category == "program" && it.Name == "rclone" && it.Severity != "error" {
			t.Errorf("required rclone missing: severity %q, want error", it.Severity)
		}
	}

	c.Tools[0].Recipes = append(c.Tools[0].Recipes, publish.InstallRecipe{Platform: "linux", Kind: "download", URL: "https://example.org/rclone.zip"})
	if _, err := CheckFolderConfig(context.Background(), t.TempDir(), "ktw-fixture", c); err == nil || !strings.Contains(err.Error(), "no sha256") {
		t.Fatalf("a download without sha256: err %v, want it refused", err)
	}
}
