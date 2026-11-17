// SPDX-License-Identifier: Apache-2.0

package file_read

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
	ErrPathFieldsBoth = errors.New("file-read: set exactly one of workspace_path or absolute_path, not both — prefer workspace_path for files inside the project")

	// ErrPathFieldsNeither signals that neither path field was set.
	ErrPathFieldsNeither = errors.New("file-read: workspace_path or absolute_path is required — prefer workspace_path for files inside the project, use absolute_path only for files outside the project (will require user approval)")

	// ErrOffsetNegative signals that the input offset was negative.
	ErrOffsetNegative = errors.New("file-read: offset must be >= 0")

	// ErrLimitNonPositive signals that the input limit was zero or
	// negative.
	ErrLimitNonPositive = errors.New("file-read: limit must be > 0 when set")

	// ErrBinaryRejected signals that the resolved path matches a
	// known binary extension and was rejected before any I/O.
	ErrBinaryRejected = errors.New("file-read: binary file types are not supported in MVP — image, PDF, and notebook reading lands in a follow-up tool")

	// ErrDevicePathRejected signals that the resolved path matches a
	// known blocking/infinite-output device and was rejected before
	// any I/O.
	ErrDevicePathRejected = errors.New("file-read: device paths are blocked")
)

// ── Validation ──────────────────────────────────────────────────────────

// Validate checks the input for shape errors that do not depend on the
// filesystem. It is called BEFORE path resolution and BEFORE any I/O.
//
// The path-field exclusivity rule is the first thing checked because
// it determines which branch the handler takes. Numeric range checks
// follow.
func Validate(in ReadInput) error {
	hasWS := in.WorkspacePath != ""
	hasAbs := in.AbsolutePath != ""
	switch {
	case hasWS && hasAbs:
		return ErrPathFieldsBoth
	case !hasWS && !hasAbs:
		return ErrPathFieldsNeither
	}
	if in.Offset != nil && *in.Offset < 0 {
		return ErrOffsetNegative
	}
	if in.Limit != nil && *in.Limit <= 0 {
		return ErrLimitNonPositive
	}
	return nil
}

// ── Path resolution ─────────────────────────────────────────────────────

// ResolvedPath holds the outcome of resolving a ReadInput against a
// PathResolver. AbsPath is always populated on success. FromAbsoluteField
// is true when the caller used absolute_path; that branch is the only
// one that needs the workspace boundary check.
type ResolvedPath struct {
	AbsPath           string
	FromAbsoluteField bool
}

// Resolve maps a validated ReadInput to an absolute filesystem path.
//
// Workspace branch (in.WorkspacePath set):
//   - delegates to resolver.Resolve, which already rejects absolute
//     inputs and `..` escapes.
//
// Absolute branch (in.AbsolutePath set):
//   - tilde expansion (~ → $HOME, ~user not supported)
//   - filepath.Clean to normalize separators
//   - filepath.Abs to anchor relative input (e.g. ".") even though
//     the field is named absolute_path; we accept any well-formed
//     OS path the user might pass and let the boundary check decide
//     whether it's authorized.
//
// Resolve does NOT EvalSymlinks. The boundary classification later
// must compare lexically against the canonical workspace root,
// otherwise a workspace-internal symlink to /etc/passwd would be
// classified as in-workspace and silently allowed.
func Resolve(in ReadInput, resolver PathResolver) (ResolvedPath, error) {
	if in.WorkspacePath != "" {
		abs, err := resolver.Resolve(in.WorkspacePath)
		if err != nil {
			return ResolvedPath{}, fmt.Errorf("file-read: resolve workspace_path: %w", err)
		}
		return ResolvedPath{AbsPath: abs, FromAbsoluteField: false}, nil
	}

	raw := in.AbsolutePath
	expanded, err := expandTilde(raw)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("file-read: expand absolute_path: %w", err)
	}
	cleaned := filepath.Clean(expanded)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("file-read: absolutize absolute_path: %w", err)
	}
	return ResolvedPath{AbsPath: abs, FromAbsoluteField: true}, nil
}

// expandTilde expands a leading ~ or ~/ to the user's home directory.
// "~user" forms are not supported and pass through unchanged (the
// subsequent absolutize step will likely fail, which is fine — it's
// not a security issue, just an unsupported form).
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
// boundary, so "/ws/foo" is in "/ws" but "/wsfoo" is not.
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
// I/O. The image/PDF/notebook entries are intentionally in the reject
// set for ms04-p1 — they will move to the supported set when
// ms05/tool-file-read-rich-formats lands.
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
	// Images (deferred to ms05)
	".png":  {},
	".jpg":  {},
	".jpeg": {},
	".gif":  {},
	".webp": {},
	".bmp":  {},
	".ico":  {},
	".tiff": {},
	// Documents (deferred to ms05)
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
// before any I/O. Reading these would either block forever or return
// infinite output. Comparisons are case-insensitive on Windows; the
// list mixes both POSIX and Windows entries because the same binary
// runs on both.
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
	// Windows reserved device names. The Windows form
	// "\\.\NUL" or just "NUL" both resolve to the device — match
	// the bare name in any cased form.
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
// blocklist entry. The Windows reserved-name check uses the cleaned
// base name (case-insensitive); the POSIX check uses the cleaned
// absolute path (case-sensitive).
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
