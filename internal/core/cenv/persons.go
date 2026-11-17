// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The person's places - the folders that hold their installations,
// registries and universes, as the process started (before a test moves HOME
// or its config folders): the config homes the global root lives under
// (~/.config, $XDG_CONFIG_HOME, %APPDATA%) and the installations' parent
// (Programs/Katyoti, ~/.local/opt/katyoti). A test process never resolves a
// root into them: KeepTestsOut gives it the same path under
// a root of its own, as the registry of installations already does
// (TestInstancesRegistry).
var personsPlaces = func() []string {
	var out []string
	add := func(p string) {
		if p == "" || !filepath.IsAbs(p) {
			return
		}
		p = filepath.Clean(p)
		for _, q := range out {
			if strings.EqualFold(p, q) {
				return
			}
		}
		out = append(out, p)
	}
	add(configHomeRoot())
	add(xdgConfigHome())
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			add(filepath.Join(la, "Programs", "Katyoti"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, ".local", "opt", "katyoti"))
	}
	// The data roots: every universe's runtime data.
	for _, d := range personsDataHomes {
		add(filepath.Join(d, "katyoti"))
	}
	return out
}()

// personsDataHomes are the data homes the person's data root lives under, as
// the process started: $XDG_DATA_HOME, %LOCALAPPDATA% and ~/.local/share.
var personsDataHomes = func() []string {
	var out []string
	if v := os.Getenv("XDG_DATA_HOME"); v != "" && filepath.IsAbs(v) {
		out = append(out, filepath.Clean(v))
	}
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" && filepath.IsAbs(la) {
			out = append(out, filepath.Clean(la))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, ".local", "share"))
	}
	return out
}()

// TestDataHome is the data home a test process and the programs it starts use
// in place of the person's: under its TestRoot. A test never writes the
// person's data root.
func TestDataHome() string { return filepath.Join(TestRoot(), "data-home") }

// isPersonsDataHome is whether a data home is one of the person's.
func isPersonsDataHome(p string) bool {
	p = filepath.Clean(p)
	for _, d := range personsDataHomes {
		if strings.EqualFold(p, d) {
			return true
		}
	}
	return false
}

// A test process starts on an empty root of its own and resolves the data
// root of its own: its TestRoot is cleared first - a pid the system reuses
// would otherwise inherit an earlier process's folder (a stale scheduler.db
// turns its worlds to the SQLite store) - then
// XDG_DATA_HOME, when unset or the person's, names TestDataHome - inherited by
// the programs it starts, so a hub or a sidecar a test runs writes its
// universes' data there too.
func init() {
	if !testing.Testing() {
		return
	}
	clearTestRoot(TestRoot())
	if v := os.Getenv("XDG_DATA_HOME"); v == "" || isPersonsDataHome(v) {
		_ = os.Setenv("XDG_DATA_HOME", TestDataHome())
	}
}

// clearTestRoot removes a test root left by an earlier process of the same
// pid. Best effort: a hub or a sidecar that process left running may still
// hold files there (Windows then refuses their removal) - what remains is
// kept, the test process starts all the same.
func clearTestRoot(root string) { _ = os.RemoveAll(root) }

// TestRoot is the folder a test process uses in place of the person's
// places: its own, in the temporary folder.
func TestRoot() string {
	return filepath.Join(os.TempDir(), "katyoti-test-root", strconv.Itoa(os.Getpid()))
}

// KeepTestsOut is p, or - in a test process, when p is or lies under one of
// the person's places - the same path under TestRoot. Every resolver of a
// root the person's installation owns (the global root, the fallback
// workspace, an installation's default folder) answers through it.
func KeepTestsOut(p string) string {
	if !testing.Testing() || p == "" {
		return p
	}
	clean := filepath.Clean(p)
	for i, place := range personsPlaces {
		if rel, ok := under(clean, place); ok {
			return filepath.Join(TestRoot(), strconv.Itoa(i), rel)
		}
	}
	return p
}

// under is p's path relative to dir when p is dir or lies under it.
func under(p, dir string) (string, bool) {
	rel, err := filepath.Rel(dir, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	if runtime.GOOS == "windows" && !strings.EqualFold(filepath.VolumeName(p), filepath.VolumeName(dir)) {
		return "", false
	}
	return rel, true
}

// isUnder is whether p is dir or lies under it.
func isUnder(p, dir string) bool {
	_, ok := under(filepath.Clean(p), filepath.Clean(dir))
	return ok
}
