// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
)

// CheckResult is the return type for custom tool check functions.
type CheckResult struct {
	OK              bool
	RequiredVersion string
	FoundVersion    string
	AdditionalInfo  string
}

// DepEntry describes the check result of an external dependency.
type DepEntry struct {
	Name            string `json:"name"`
	RequiredVersion string `json:"required_version,omitempty"`
	FoundVersion    string `json:"found_version,omitempty"`
	OK              bool   `json:"ok"`
	InstallSkill    string `json:"installSkill,omitempty"` // skill URI for install instructions
	AdditionalInfo  string `json:"additional_info,omitempty"`
}

// CheckTool runs the detection logic for a ToolDef and returns a DepEntry.
// Returns nil if the tool has nothing to check (no Commands, no Check func).
//
// The ctx parameter is passed to custom Check functions so they can access
// the Application (via application.AppFromContext) for config lookups.
//
// Check logic:
//  1. Commands set → LookPath → found + VersionFlag set → run {binary} {flag} → DepEntry with version;
//     with Version set, a different found version is not OK (RequiredVersion says which)
//  2. Commands set → LookPath → found, no VersionFlag → DepEntry{OK: true}
//  3. Check set (no Commands) → call it → DepEntry from result
//  4. Neither → skip (returns nil)
//
// A PythonPackage is checked as that package (CheckPythonPackage), before all
// of these.
func CheckTool(t ToolDef, ctx context.Context) *DepEntry {
	var check func(context.Context) CheckResult
	switch {
	case t.PythonPackage != "":
		check = func(context.Context) CheckResult { return CheckPythonPackage(t.PythonPackage) }
	case len(t.Commands) == 0 && t.Check != nil:
		// Custom check function takes priority when no Commands are set.
		fn, ok := t.Check.(func(context.Context) CheckResult)
		if !ok {
			return nil
		}
		check = fn
	}
	if check != nil {
		r := check(ctx)
		entry := &DepEntry{
			Name:            t.Name,
			OK:              r.OK,
			RequiredVersion: r.RequiredVersion,
			FoundVersion:    r.FoundVersion,
			AdditionalInfo:  r.AdditionalInfo,
		}
		if !r.OK && t.InstallSkill != "" {
			entry.InstallSkill = t.InstallSkill
		}
		return entry
	}

	// Commands-based auto-check via LookPath. A command that answers its
	// version flag with words and no version is not the program (the Windows
	// Store's python stub answers "Python was not found"): the next is tried.
	if len(t.Commands) > 0 {
		for _, cmd := range t.Commands {
			if p, err := exec.LookPath(cmd); err == nil {
				entry := &DepEntry{Name: t.Name, OK: true, RequiredVersion: t.Version}
				if t.VersionFlag != "" {
					if ver, err := CheckToolVersion(p, t.VersionFlag); err == nil {
						if ver != "" && !strings.ContainsAny(ver, "0123456789") {
							continue
						}
						entry.FoundVersion = ver
						if !VersionMatches(ver, t.Version) {
							entry.OK = false
							entry.InstallSkill = t.InstallSkill
						}
					}
				}
				return entry
			}
		}
		return &DepEntry{Name: t.Name, InstallSkill: t.InstallSkill}
	}

	return nil // nothing to check
}

// VersionMatches reports whether a found version is the validated one: found
// starts with want and the next character does not continue a number
// ("0.15.1 (9dfd3a08)" matches "0.15.1"; "3.110" does not match "3.11").
// An empty want matches anything.
func VersionMatches(found, want string) bool {
	if want == "" {
		return true
	}
	if !strings.HasPrefix(found, want) {
		return false
	}
	rest := found[len(want):]
	return rest == "" || !(rest[0] >= '0' && rest[0] <= '9' || rest[0] == '.')
}

// CheckToolVersion runs "{path} {flag}" with stdin closed, captures stdout and stderr,
// and returns the first line stripped of any leading non-digit prefix.
// Stdin is explicitly closed so GUI/Electron apps don't hang waiting for input.
func CheckToolVersion(path, flag string) (string, error) {
	cmd := exec.Command(path, flag)
	NoConsoleWindow(cmd)
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	// Ignore exit code — some apps (e.g. Electron) print the version but exit non-zero.
	_ = cmd.Run()
	line := strings.TrimSpace(strings.SplitN(out.String(), "\n", 2)[0])
	if line == "" {
		return "", nil
	}
	return TrimVersionPrefix(line), nil
}

// TrimVersionPrefix strips all leading characters before the first ASCII digit.
// "typst 0.14.2 (b33de9de)" → "0.14.2 (b33de9de)", "pandoc.exe 3.6.4" → "3.6.4".
func TrimVersionPrefix(s string) string {
	for i, c := range s {
		if c >= '0' && c <= '9' {
			return s[i:]
		}
	}
	return s
}

// FindInPath checks well-known install directories for a program.
// paths is a list of candidate absolute paths to check.
// Returns the first path that exists, or empty string.
// Use this in custom Check functions for tools that install outside PATH.
func FindInPath(paths ...string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// IsVersionString returns true for strings like "124.0.6367.207".
func IsVersionString(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// ChromeVersionFromDir reads the Chrome version from the version-named subdirectory
// inside the Application folder (e.g. "Application/124.0.6367.207/").
func ChromeVersionFromDir(appDir string) string {
	entries, err := os.ReadDir(appDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if IsVersionString(e.Name()) {
			return e.Name()
		}
	}
	return ""
}
