// SPDX-License-Identifier: Apache-2.0

// Package pathtool provides shared helpers for file-touching agent
// tools (file-read, file-write, file-edit, glob, grep). It eliminates
// the ~185 lines of code that were previously duplicated across
// file-read and file-write — the refactor was triggered when
// file-edit became the third consumer (see the refactor-trigger
// notes in file-read/validate.go and file-write/validate.go).
//
// The package is pure — no DI, no service registration, no
// application context. It receives parameters and returns values.
// This keeps it importable from any tool package without cycles.
//
// Contents:
//   - PathResolver: the interface every file tool requires from
//     the workspace
//   - Resolve + ResolvedPath + expandTilde: workspace vs absolute
//     path resolution (mutex-fields pattern)
//   - ClassifyAgainstWorkspace: lexical separator-aware boundary check
//   - IsBinaryExtension + IsBlockedDevicePath: defensive blocklists
//   - BuildOutsideWorkspaceSuggestions: graduated rule suggestions for
//     the permission prompt (parameterized by tool name)
package pathtool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// PathResolver resolves a workspace-relative path to an absolute
// path AND exposes the workspace root so tools can classify
// absolute_path inputs against the workspace boundary.
//
// Every file-touching tool defines its own duck-typed interface in
// its package for backward compatibility. This interface is the
// canonical version; the per-tool copies are identical.
type PathResolver interface {
	Resolve(relPath string) (string, error)
	WorkspaceRoot() string
}

// ResolvedPath holds the outcome of resolving a tool input to an
// absolute filesystem path. AbsPath is always populated on success.
// FromAbsoluteField is true when the caller used the absolute_path
// field; that branch is the only one that needs the workspace
// boundary check.
type ResolvedPath struct {
	AbsPath           string
	FromAbsoluteField bool
}

// Resolve maps a workspace-relative OR absolute path to an absolute
// filesystem path. Exactly one of wsPath and absPath must be non-empty
// (the caller's Validate already enforces the mutex).
//
// Workspace branch (wsPath non-empty):
//   - delegates to resolver.Resolve, which rejects absolute inputs
//     and `..` escapes.
//
// Absolute branch (absPath non-empty):
//   - tilde expansion (~ → $HOME, ~user not supported)
//   - filepath.Clean to normalize separators
//   - filepath.Abs to anchor relative input
//
// Resolve does NOT EvalSymlinks. The boundary classification later
// must compare lexically against the canonical workspace root.
func Resolve(wsPath, absPath string, resolver PathResolver, toolName string) (ResolvedPath, error) {
	if wsPath != "" {
		abs, err := resolver.Resolve(wsPath)
		if err != nil {
			return ResolvedPath{}, fmt.Errorf("%s: resolve workspace_path: %w", toolName, err)
		}
		return ResolvedPath{AbsPath: abs, FromAbsoluteField: false}, nil
	}

	raw := absPath
	expanded, err := ExpandTilde(raw)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("%s: expand absolute_path: %w", toolName, err)
	}
	cleaned := filepath.Clean(expanded)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("%s: absolutize absolute_path: %w", toolName, err)
	}
	return ResolvedPath{AbsPath: abs, FromAbsoluteField: true}, nil
}

// ExpandTilde expands a leading ~ or ~/ to the user's home directory.
// "~user" forms are not supported and pass through unchanged.
func ExpandTilde(p string) (string, error) {
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
}

// ClassifyAgainstWorkspace reports whether absPath is inside the
// workspace rooted at workspaceRoot.
//
// Both arguments must already be absolute and lexically clean.
// The check uses lexical prefix matching with a separator-aware
// boundary, so "/ws/foo" is in "/ws" but "/wsfoo" is not.
//
// The workspace root SHOULD be canonicalized once at session start;
// the target absPath MUST NOT be canonicalized (that would let
// workspace-internal symlinks pointing outside be misclassified).
func ClassifyAgainstWorkspace(absPath, workspaceRoot string) bool {
	if workspaceRoot == "" {
		return false // fail-closed
	}
	rootClean := filepath.Clean(workspaceRoot)
	pathClean := filepath.Clean(absPath)
	if pathClean == rootClean {
		return true
	}
	return strings.HasPrefix(pathClean, rootClean+string(filepath.Separator))
}

// ── Defensive blocklists ────────────────────────────────────────────────

// binaryExtensions is the set of file extensions rejected before any
// I/O. All file tools share this blocklist.
var binaryExtensions = map[string]struct{}{
	".exe": {}, ".dll": {}, ".so": {}, ".dylib": {},
	".o": {}, ".obj": {}, ".a": {}, ".lib": {},
	".bin": {}, ".class": {}, ".pyc": {}, ".pyo": {},
	".zip": {}, ".tar": {}, ".gz": {}, ".bz2": {}, ".7z": {}, ".rar": {},
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {},
	".bmp": {}, ".ico": {}, ".tiff": {},
	".pdf": {}, ".ipynb": {},
	".doc": {}, ".docx": {}, ".xls": {}, ".xlsx": {}, ".ppt": {}, ".pptx": {},
	".mp3": {}, ".mp4": {}, ".wav": {}, ".ogg": {}, ".mov": {}, ".avi": {}, ".mkv": {},
}

// IsBinaryExtension reports whether absPath has an extension in the
// rejected set. Comparison is case-insensitive.
func IsBinaryExtension(absPath string) bool {
	ext := strings.ToLower(filepath.Ext(absPath))
	if ext == "" {
		return false
	}
	_, ok := binaryExtensions[ext]
	return ok
}

// blockedDevicePaths is the set of OS-specific device paths rejected
// before any I/O.
var blockedDevicePaths = map[string]struct{}{
	"/dev/zero": {}, "/dev/random": {}, "/dev/urandom": {},
	"/dev/null": {}, "/dev/stdin": {}, "/dev/stdout": {},
	"/dev/stderr": {}, "/dev/tty": {},
	"nul": {}, "con": {}, "prn": {}, "aux": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {},
}

// IsBlockedDevicePath reports whether absPath matches a known device
// blocklist entry.
func IsBlockedDevicePath(absPath string) bool {
	cleaned := filepath.Clean(absPath)
	if _, ok := blockedDevicePaths[cleaned]; ok {
		return true
	}
	base := strings.ToLower(filepath.Base(cleaned))
	if dot := strings.Index(base, "."); dot >= 0 {
		base = base[:dot]
	}
	_, ok := blockedDevicePaths[base]
	return ok
}

// ── Permission suggestions ──────────────────────────────────────────────

// BuildOutsideWorkspaceSuggestions returns a graduated set of
// "always allow" suggestions for an out-of-workspace tool call.
// Three levels:
//
//  1. Allow this exact file only (narrowest, safest)
//  2. Allow any operation under the parent directory
//  3. Allow any out-of-workspace operation by this tool (broadest)
//
// The toolName parameter determines the rule prefix (e.g.
// "file-read", "file-write", "file-edit") so each tool's rules
// are independent in the permission rules system.
//
// The slashAbs argument MUST be the absolute path with forward
// slashes (via filepath.ToSlash) so the resulting rule strings are
// portable across operating systems.
func BuildOutsideWorkspaceSuggestions(toolName, slashAbs string) []toolsapi.PermissionSuggestion {
	parent := slashAbs
	if i := strings.LastIndex(slashAbs, "/"); i >= 0 {
		parent = slashAbs[:i]
	}
	return []toolsapi.PermissionSuggestion{
		{
			Label: fmt.Sprintf("Always allow this exact file (%s)", slashAbs),
			Rule:  toolName + ".absolute_path:" + slashAbs,
		},
		{
			Label: fmt.Sprintf("Always allow under %s", parent),
			Rule:  toolName + ".absolute_path:" + parent + "/**",
		},
		{
			Label: fmt.Sprintf("Always allow any out-of-workspace operation by %s", toolName),
			Rule:  toolName + ".absolute_path:**",
		},
	}
}

// AttachPathHintIfNeeded adds a hint warning when the LLM used
// absolute_path for a file that turned out to be inside the
// workspace. The hint does NOT fire for outside-workspace paths
// (the user explicitly approved those via the permission gate).
func AttachPathHintIfNeeded(warning string, resolved ResolvedPath, resolver PathResolver) string {
	if !resolved.FromAbsoluteField {
		return warning
	}
	if !ClassifyAgainstWorkspace(resolved.AbsPath, resolver.WorkspaceRoot()) {
		return warning
	}
	hint := "this file is inside the workspace; prefer 'workspace_path' next time to skip the permission check"
	if warning == "" {
		return hint
	}
	return warning + "; " + hint
}
