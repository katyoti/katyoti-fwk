// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// status names each problem of the installation with its file and path, and
// says the doctor repairs them; a sound installation says so.
func TestWriteDiagnosis(t *testing.T) {
	var sound bytes.Buffer
	writeDiagnosis(&sound, mktapi.InstallationDiagnosis{})
	if !strings.Contains(sound.String(), "sound") {
		t.Fatalf("a sound installation: %q", sound.String())
	}
	var b bytes.Buffer
	writeDiagnosis(&b, mktapi.InstallationDiagnosis{Problems: []mktapi.InstallationProblem{
		{Kind: mktapi.ProblemProgramReplaced, File: "/cp/ktw-hub", Entry: "ktw-hub", Path: "/cp/ktw-hub", Detail: "program \"ktw-hub\" 2.0 was replaced"},
		{Kind: mktapi.ProblemVanishedPath, File: "/root/universes.yaml", Entry: "old", Path: "/gone/old", Detail: "universe \"old\" names a path that is gone"},
	}})
	out := b.String()
	for _, want := range []string{"2 problem(s)", "katyoti doctor", "program-replaced", "/cp/ktw-hub", "vanished-path", "/root/universes.yaml", "path: /gone/old"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output lacks %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "path: /cp/ktw-hub") != 0 {
		t.Errorf("a program's path is said once, as its file:\n%s", out)
	}
}

// status counts a dev installation's programs as the dev builds they are -
// one each, a store copy kept beside said apart - and leaves the control
// plane to its own section.
func TestWriteInstalledCountsADevInstallation(t *testing.T) {
	dev := &mktapi.DevBuild{Version: "c4ea4719-dirty"}
	var b bytes.Buffer
	writeInstalledCounts(&b, []mktapi.Installed{
		{Name: "ktw-hub", Source: "executable-dir", ControlPlane: true},
		{Name: "ktw-agents", Source: "marketplace", Version: "v2026.10-rc5", DevBuild: dev},
		{Name: "ktw-code", Source: "executable-dir", DevBuild: dev},
		{Name: "ktw-claude", Source: "executable-dir", DevBuild: dev},
	})
	out := b.String()
	for _, want := range []string{"3 dev builds", "make dev updates them", "1 also kept in the store"} {
		if !strings.Contains(out, want) {
			t.Errorf("status lacks %q:\n%s", want, out)
		}
	}
	for _, not := range []string{"managed", "discovered"} {
		if strings.Contains(out, not) {
			t.Errorf("a dev installation's programs are counted %s:\n%s", not, out)
		}
	}

	b.Reset()
	writeInstalledCounts(&b, []mktapi.Installed{{Name: "ktw-agents", Source: "marketplace"}, {Name: "ktw-x", Source: "executable-dir"}})
	if got := b.String(); !strings.Contains(got, "1 managed, 1 discovered") || strings.Contains(got, "dev builds") {
		t.Errorf("a stable installation's counts: %q", got)
	}
}

// An infra update says what it moved; one that replaced the dev build make dev
// placed says so, never « (absent) ».
func TestWriteInfraUpdatesSaysTheDevBuildItReplaced(t *testing.T) {
	var dev, channel bytes.Buffer
	WriteInfraUpdates(&dev, "infra: ", []mktapi.Update{{Name: "katyoti", Installed: "c4ea4719-dirty", Latest: "v2026.10-rc7"}}, true)
	if got := dev.String(); got != "infra: katyoti dev build c4ea4719-dirty (make dev) replaced by v2026.10-rc7\n" {
		t.Errorf("dev build replaced: %q", got)
	}
	WriteInfraUpdates(&channel, "infra: ", []mktapi.Update{{Name: "katyoti", Latest: "v2"}, {Name: "ktw-hub", Installed: "v1", Latest: "v2"}}, false)
	if got := channel.String(); got != "infra: katyoti (absent) -> v2\ninfra: ktw-hub v1 -> v2\n" {
		t.Errorf("channel update: %q", got)
	}
}
