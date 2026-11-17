// SPDX-License-Identifier: Apache-2.0

package file_write

// This file is intentionally a near-copy of file_read's validate.go.
// The validation, path resolution, classifier, and blocklist helpers
// are identical in shape and almost identical in content; only the
// content-size cap (FileWriteConfig.MaxBytes) is file_write-specific.
//
// **Why duplicated, not extracted**: file_write is the second
// consumer of these helpers. Extracting to a shared
// `tools/api/pathtool/` package now risks getting the abstraction
// wrong with only two data points. The refactor trigger is the
// THIRD consumer (tool-file-edit). When that lands, the duplicated
// helpers across file_read, file_write, and file_edit get
// collapsed to a single shared package called from each tool.
//
// Until then: duplication is cheap, the cost is ~185 lines, and
// the risk of premature abstraction is real.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ── Sentinel errors ─────────────────────────────────────────────────────
//
// Validation errors are sentinels (not wrapped) so tests can use
// errors.Is() and the handler can branch on them when shaping the
// returned tool error message.

var (
	// ErrPathFieldsBoth signals that both workspace_path and absolute_path
	// were set on the same call. The validation message nudges toward
	// workspace_path as the safe default.
	ErrPathFieldsBoth = errors.New("file-write: set exactly one of workspace_path or absolute_path, not both — prefer workspace_path for files inside the project")

	// ErrPathFieldsNeither signals that neither path field was set.
	ErrPathFieldsNeither = errors.New("file-write: workspace_path or absolute_path is required — prefer workspace_path for files inside the project, use absolute_path only for files outside the project (will require user approval)")

	// ErrContentTooLarge signals that the input content exceeded the
	// configured cap.
	ErrContentTooLarge = errors.New("file-write: content exceeds the configured size cap")

	// ErrBinaryRejected signals that the resolved path matches a
	// known binary extension and was rejected before any I/O.
	ErrBinaryRejected = errors.New("file-write: binary file types are not supported — file-write is for text content only")

	// ErrDevicePathRejected signals that the resolved path matches a
	// known blocking/infinite-output device and was rejected before
	// any I/O.
	ErrDevicePathRejected = errors.New("file-write: device paths are blocked")
)

// ── Validation ──────────────────────────────────────────────────────────

// Validate checks the input for shape errors that do not depend on
// the filesystem. It is called BEFORE path resolution and BEFORE any
// I/O.
//
// The maxBytes argument comes from FileWriteConfig.MaxBytes (resolved
// per-call from context) so tests can shrink the cap to exercise the
// rejection path without writing megabytes.
//
// The path-field exclusivity rule is checked first because it
// determines which branch the handler takes. Content size is checked
// second.
//
// Empty content is allowed: creating a placeholder file is a
// legitimate use case (touch a `.gitkeep`, write an empty config
// that gets populated later).
func Validate(in WriteInput, maxBytes int) error {
	hasWS := in.WorkspacePath != ""
	hasAbs := in.AbsolutePath != ""
	switch {
	case hasWS && hasAbs:
		return ErrPathFieldsBoth
	case !hasWS && !hasAbs:
		return ErrPathFieldsNeither
	}
	if maxBytes > 0 && len(in.Content) > maxBytes {
		return fmt.Errorf("%w: %d bytes (cap %d)", ErrContentTooLarge, len(in.Content), maxBytes)
	}
	return nil
}

// ── Path resolution ─────────────────────────────────────────────────────

// ResolvedPath holds the outcome of resolving a WriteInput against a
// PathResolver. AbsPath is always populated on success.
// FromAbsoluteField is true when the caller used absolute_path; that
// branch is the only one that needs the workspace boundary check.
type ResolvedPath struct {
	AbsPath           string
	FromAbsoluteField bool
}

// Resolve maps a validated WriteInput to an absolute filesystem path.
//
// Workspace branch (in.WorkspacePath set):
//   - delegates to resolver.Resolve, which already rejects absolute
//     inputs and `..` escapes.
//
// Absolute branch (in.AbsolutePath set):
//   - tilde expansion (~ → $HOME, ~user not supported)
//   - filepath.Clean to normalize separators
//   - filepath.Abs to anchor relative input even though the field is
//     named absolute_path; we accept any well-formed OS path the
//     user might pass and let the boundary check decide whether
//     it's authorized.
//
// Resolve does NOT EvalSymlinks. The boundary classification later
// must compare lexically against the canonical workspace root,
// otherwise a workspace-internal symlink to /etc/passwd would be
// classified as in-workspace and silently allowed.
func Resolve(in WriteInput, resolver PathResolver) (ResolvedPath, error) {
	if in.WorkspacePath != "" {
		abs, err := resolver.Resolve(in.WorkspacePath)
		if err != nil {
			return ResolvedPath{}, fmt.Errorf("file-write: resolve workspace_path: %w", err)
		}
		return ResolvedPath{AbsPath: abs, FromAbsoluteField: false}, nil
	}

	raw := in.AbsolutePath
	expanded, err := expandTilde(raw)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("file-write: expand absolute_path: %w", err)
	}
	cleaned := filepath.Clean(expanded)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("file-write: absolutize absolute_path: %w", err)
	}
	return ResolvedPath{AbsPath: abs, FromAbsoluteField: true}, nil
}

// expandTilde expands a leading ~ or ~/ to the user's home directory.
// "~user" forms are not supported and pass through unchanged.
func expandTilde(p string) (string, error) {
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

// ── Workspace boundary classification ───────────────────────────────────

// ClassifyAgainstWorkspace reports whether absPath is inside the
// workspace rooted at workspaceRoot.
//
// Both arguments must already be absolute and lexically clean. The
// workspace root SHOULD be canonicalized (via filepath.EvalSymlinks)
// once at session start; the target absPath MUST NOT be canonicalized
// — that would let workspace-internal symlinks pointing outside the
// workspace be misclassified as in-workspace.
//
// The check uses lexical prefix matching with a separator-aware
// boundary, so "/ws/foo" is in "/ws" but "/wsfoo" is not (the
// `/wsfoo` trap covered by tests).
func ClassifyAgainstWorkspace(absPath, workspaceRoot string) (insideWorkspace bool) {
	if workspaceRoot == "" {
		// No workspace root configured → conservatively classify as
		// outside, which forces the permission ask. Better to over-
		// prompt than to leak.
		return false
	}
	rootClean := filepath.Clean(workspaceRoot)
	pathClean := filepath.Clean(absPath)

	if pathClean == rootClean {
		return true
	}
	rootWithSep := rootClean + string(filepath.Separator)
	return strings.HasPrefix(pathClean, rootWithSep)
}

// ── Defensive blocklists ────────────────────────────────────────────────

// binaryExtensions is the set of file extensions rejected before any
// I/O. file_write is for text content only — image, PDF, archive,
// and binary executable writes don't make sense (the LLM doesn't
// produce binary content as a string).
//
// Extensions are stored lowercase including the leading dot.
var binaryExtensions = map[string]struct{}{
	// Executables and object code
	".exe":   {},
	".dll":   {},
	".so":    {},
	".dylib": {},
	".o":     {},
	".obj":   {},
	".a":     {},
	".lib":   {},
	".bin":   {},
	".class": {},
	".pyc":   {},
	".pyo":   {},
	// Archives
	".zip": {},
	".tar": {},
	".gz":  {},
	".bz2": {},
	".7z":  {},
	".rar": {},
	// Images (binary content; LLM can't produce these as text)
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".gif":  {},
	".webp": {},
	".bmp":  {},
	".ico":  {},
	".tiff": {},
	// Documents (binary or proprietary)
	".pdf":   {},
	".ipynb": {},
	".doc":   {},
	".docx":  {},
	".xls":   {},
	".xlsx":  {},
	".ppt":   {},
	".pptx":  {},
	// Media
	".mp3": {},
	".mp4": {},
	".wav": {},
	".ogg": {},
	".mov": {},
	".avi": {},
	".mkv": {},
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
// before any I/O. Writing to these would either block forever or
// corrupt system state.
var blockedDevicePaths = map[string]struct{}{
	// POSIX-style
	"/dev/zero":    {},
	"/dev/random":  {},
	"/dev/urandom": {},
	"/dev/null":    {},
	"/dev/stdin":   {},
	"/dev/stdout":  {},
	"/dev/stderr":  {},
	"/dev/tty":     {},
	// Windows reserved device names.
	"nul":  {},
	"con":  {},
	"prn":  {},
	"aux":  {},
	"com1": {},
	"com2": {},
	"com3": {},
	"com4": {},
	"lpt1": {},
	"lpt2": {},
	"lpt3": {},
}

// IsBlockedDevicePath reports whether absPath matches a known device
// blocklist entry.
func IsBlockedDevicePath(absPath string) bool {
	cleaned := filepath.Clean(absPath)
	if _, ok := blockedDevicePaths[cleaned]; ok {
		return true
	}
	// Windows: also reject the bare reserved-name forms regardless
	// of case and any extension (NUL, NUL.txt, nul, etc).
	base := strings.ToLower(filepath.Base(cleaned))
	if dot := strings.Index(base, "."); dot >= 0 {
		base = base[:dot]
	}
	if _, ok := blockedDevicePaths[base]; ok {
		return true
	}
	return false
}
