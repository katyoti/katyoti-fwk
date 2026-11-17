// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"debug/buildinfo"
	"regexp"
	"runtime/debug"
	"strings"
)

// ResolveVersion returns a meaningful version string for a binary.
//
//   - If `compiled` was stamped at build time (anything other than the "dev"
//     default), it wins — e.g. a release build with `-ldflags "-X …cmd.version=1.2.3"`.
//   - Otherwise it derives from Go's automatic VCS build info — the short
//     commit (+ "-dirty" when the working tree was modified) — so a plain
//     `go build` / `make` binary reports its git revision instead of "dev".
//   - Falls back to "dev" only when there is no VCS info at all.
//
// Build-tool-agnostic: relies on the toolchain's built-in VCS stamping (Go
// 1.18+), so it works regardless of the make implementation.
func ResolveVersion(compiled string) string {
	if v := strings.TrimSpace(compiled); v != "" && v != "dev" {
		return v
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	return vcsVersion(info.Settings)
}

// ProgramVersion reads the version the Go program at path reports, from its
// build info on disk - without running it (a run costs an antivirus scan, and
// ktw-gui would open a window). stamped says whether a release stamped it
// (`-X …version=<v>`, what `make release` does); an unstamped program is a
// development build - make build, make dev - and its version is its commit,
// as ResolveVersion derives it. An error: not a Go program, or unreadable.
func ProgramVersion(path string) (version string, stamped bool, err error) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	version, stamped = buildVersion(info.Settings)
	return version, stamped, nil
}

// buildVersion is the version a build's settings give: the release stamp of
// its -ldflags when there is one, else its commit.
func buildVersion(settings []debug.BuildSetting) (string, bool) {
	for _, s := range settings {
		if s.Key != "-ldflags" {
			continue
		}
		if m := versionStamp.FindStringSubmatch(s.Value); m != nil && m[1] != "dev" {
			return m[1], true
		}
	}
	return vcsVersion(settings), false
}

// versionStamp finds the version a build stamped with -X: `cmd.version=<v>`
// for the Go programs, `main.version=<v>` for ktw-gui.
var versionStamp = regexp.MustCompile(`\.version=([^\s'"]+)`)

// vcsVersion is the short commit of a build's VCS settings, "-dirty" when its
// tree was modified, "dev" when it has none.
func vcsVersion(settings []debug.BuildSetting) string {
	var rev string
	var dirty bool
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "dev"
	}
	if len(rev) > 8 {
		rev = rev[:8]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}
