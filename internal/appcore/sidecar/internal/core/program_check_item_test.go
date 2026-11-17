// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// A tool found at another version than the validated one is "outdated" (a
// warning with both versions and the install skill), not "missing".
func TestProgramCheckItemOutdated(t *testing.T) {
	tool := publish.ToolDef{Name: "typst", InstallSkill: "ktw-docs:/skill/install-typst"}
	item := programCheckItem(tool, api.ToolDescriptor{FoundVersion: "0.12.0 (737895d7)", RequiredVersion: "0.15.1"})
	if item.Status != "outdated" || item.Severity != "warn" {
		t.Fatalf("status %q severity %q, want outdated / warn", item.Status, item.Severity)
	}
	if item.Detail != "found 0.12.0 (737895d7); validated with 0.15.1" {
		t.Fatalf("detail %q", item.Detail)
	}
	if item.Hint == "" {
		t.Fatal("no hint to the install skill")
	}
	if missing := programCheckItem(tool, api.ToolDescriptor{}); missing.Status != "missing" {
		t.Fatalf("not found: status %q, want missing", missing.Status)
	}
}
