// SPDX-License-Identifier: Apache-2.0

// Package controlplane resolves the control-plane location — the stable directory
// holding katyoti's own infra binaries (katyoti + ktw-hub + ktw-gui + ktw-univers).
//
// It is kept SEPARATE from the versioned sidecar store so the launcher's pre-hub
// direct install/swap never races the hub (universe-mgmt-model-and-design DO.11).
// Ordinary sidecars run under the hub from the versioned store and are add-version
// + repoint; the infra binaries run from a fixed path and so must be swapped
// in place (selfupdate.Replace) — they are the only ones with the running-exe
// problem.
package controlplane

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/katyoti/katyoti-fwk/internal/appcore/selfupdate"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// AdminUniverse is the reserved name of the control-plane universe — the one
// that serves the management services. It is infrastructure, not a user
// selection: callers may activate it on demand where they would refuse to
// auto-spawn any other universe.
const AdminUniverse = "admin"

// AdminHome is the admin universe's home — `<GlobalHomeDir>/universes/admin/`,
// its deterministic location across the system.
func AdminHome() string {
	return filepath.Join(application.Env().GlobalHomeDir(), "universes", AdminUniverse)
}

// InfraBinaries are the control-plane binaries katyoti installs + self-updates.
//
// ktw-univers is one of them, and belongs here for the same reasons as the other
// three rather than as a convenience:
//
//   - It IS the control plane. The admin universe's hub serves the marketplace
//     and orchestrator slots out of ktw-univers; without it admin comes up with
//     no management surface at all and every GUI call times out.
//   - It moves as a SET with the hub. They speak one KTSP protocol version to
//     each other, so shipping them independently is a skew bug you build by hand.
//   - It cannot come from a catalog, because it is the service that SERVES the
//     catalog. Installing it from the marketplace is circular on a bare machine.
//
// Before this it was in neither list: not in the infra channel, not installed
// from the catalog, just named in admin's manifest and left to PATH. That worked
// only on a machine that happened to have a dev install on PATH — which also
// meant a "self-contained" KTW_KATYOTI_ROOT instance silently borrowed a binary
// from outside its root. On a machine built from the published channel alone it
// was simply absent.
var InfraBinaries = []string{"katyoti", "ktw-hub", "ktw-gui", "ktw-univers"}

// Dir is the control-plane location — `<GlobalHomeDir>/control-plane`.
func Dir() (string, error) {
	g := application.Env().GlobalHomeDir()
	if g == "" {
		return "", fmt.Errorf("controlplane: global home dir unresolved")
	}
	return filepath.Join(g, "control-plane"), nil
}

// TargetPath is the stable path of one control-plane binary — `<Dir>/<name><exe>`.
// The update flow lands a new version in the versioned store, then swaps it into
// this path with selfupdate.Replace.
func TargetPath(name string) (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name+exeSuffix()), nil
}

// DevDir is where the dev channel's other programs are installed -
// `<GlobalHomeDir>/bin/dev` (make install), beside the control plane.
func DevDir() (string, error) {
	g := application.Env().GlobalHomeDir()
	if g == "" {
		return "", fmt.Errorf("controlplane: global home dir unresolved")
	}
	return filepath.Join(g, "bin", "dev"), nil
}

// ProgramPath is where the installation keeps the program name as installed
// from a checkout: the control plane for one of InfraBinaries, DevDir else.
func ProgramPath(name string) (string, error) {
	if IsInfra(name) {
		return TargetPath(name)
	}
	d, err := DevDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, name+exeSuffix()), nil
}

// IsInfra reports whether name is one of the control-plane binaries.
func IsInfra(name string) bool {
	for _, n := range InfraBinaries {
		if n == name {
			return true
		}
	}
	return false
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// Sweep reaps stale `<binary>.old-*` files left beside the control-plane binaries
// by a swap that could not delete the outgoing file — on Windows a RUNNING exe can
// be renamed aside but not removed, so the previous version lingers until a later
// process cleans it up.
//
// Best-effort and safe to call on every startup: files still locked (including the
// currently-running binary's own predecessor) are skipped and reaped on a later
// run. Returns the first scan error, if any; individual removal failures are not
// errors.
func Sweep() error {
	var firstErr error
	for _, name := range InfraBinaries {
		target, err := TargetPath(name)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := selfupdate.CleanupOld(target); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
