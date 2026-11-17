// SPDX-License-Identifier: Apache-2.0

package prereqs

import (
	"context"
	"strings"
	"testing"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// pymupdfReport is a sidecar's config check of the pip package pymupdf, as
// its check (publish.CheckTool, the package read as a package) answered.
func pymupdfReport(found bool) sidecarapi.ConfigCheckReport {
	d := sidecarapi.NewToolDescriptor(publish.ToolDef{Name: "pymupdf", PythonPackage: "pymupdf",
		Recipes: []publish.InstallRecipe{pip("windows", "pymupdf")}})
	d.OK = found
	if found {
		d.FoundVersion = "1.24.10"
	}
	return sidecarapi.ConfigCheckReport{Deps: []sidecarapi.ToolDescriptor{d}}
}

// A pip package installed and found by its package check is reported found
// by the post-install recheck, as the Python package it is - never as a
// command looked for on PATH.
func TestInstall_APipPackageFoundByItsPackageCheckIsFoundAfterTheInstall(t *testing.T) {
	f := newFake()
	e := &Engine{Runner: f, Platform: "windows"}
	before := e.Plan(FromConfigCheck("kb", "", pymupdfReport(false)))
	if p := program(t, before, "pymupdf"); p.State != mktapi.PrereqInstall || p.PythonPackage != "pymupdf" {
		t.Fatalf("before: pymupdf %s, python package %q - want install, pymupdf", p.State, p.PythonPackage)
	}
	recheck := func(context.Context) ([]Need, []string) { return FromConfigCheck("kb", "", pymupdfReport(true)), nil }
	out, err := e.Install(context.Background(), before, nil, false, recheck)
	if err != nil {
		t.Fatal(err)
	}
	if out.After == nil {
		t.Fatal("no recheck after the install")
	}
	p := program(t, *out.After, "pymupdf")
	if p.State != mktapi.PrereqOK || p.Found != "1.24.10" || p.PythonPackage != "pymupdf" || len(p.Commands) > 0 {
		t.Errorf("after: pymupdf %s found %q python package %q commands %v - want ok, 1.24.10, the package, no command",
			p.State, p.Found, p.PythonPackage, p.Commands)
	}
	if len(out.Missing) != 0 {
		t.Errorf("missing %v after the package was found", out.Missing)
	}
}

// A Python package no recipe installs here is to install as a package: its
// fallback never sends the person to the PATH.
func TestPlan_APythonPackageFallbackNeverSpeaksOfThePath(t *testing.T) {
	needs := FromConfigCheck("kb", "", pymupdfReport(false))
	needs[0].Commands = []string{"pymupdf"} // even a declaration that also names a command
	p := program(t, Plan(needs, "windows", noPip), "pymupdf")
	if p.State != mktapi.PrereqManual {
		t.Fatalf("pymupdf %s, want manual", p.State)
	}
	if strings.Contains(p.Fallback, "PATH") || !strings.Contains(p.Fallback, "Python package pymupdf") {
		t.Errorf("fallback %q: want the Python package, never the PATH", p.Fallback)
	}
}
