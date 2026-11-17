// SPDX-License-Identifier: Apache-2.0

// Package file_write implements the file_write agent tool.
//
// See schema.go for the package-level documentation, file layout,
// and MVP scope notes.
package file_write

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/filestate"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

// NewFileWriteTool creates a file_write tool wired to the given
// path resolver. The resolver MUST satisfy PathResolver — it must
// expose both Resolve and WorkspaceRoot.
//
// The tool's config and per-session filestate Store are resolved
// per-call from context, NOT captured at construction. This lets:
//   - tests override the config without rebuilding the tool
//   - per-session filestate stores work correctly
//   - the same tool instance serve multiple sessions safely
//
// The PathResolver IS captured because it represents the workspace
// boundary, which is a tool-construction-time concern (one workspace
// per tool instance).
func NewFileWriteTool(resolver PathResolver) toolsapi.Tool {
	handler := func(ctx context.Context, in WriteInput) (WriteOutput, error) {
		return runWrite(ctx, in, resolver)
	}
	return toolsapi.NewFunctionTool(
		"file-write",
		toolDescription,
		handler,
		toolsapi.WithMutating(),
		toolsapi.WithPermissionCheck(makePermissionFunc(resolver)),
		toolsapi.WithDescriber(describeWrite),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📝", Label: "Write", Color: "#C3E88D", Template: "{{.file_path}}"}),
	)
}

// runWrite is the full handler body. It re-runs Validate and Resolve
// (the permission gate already did so without the size cap, but the
// handler must enforce it; re-running is cheap and the gate result
// is not threaded through).
//
// Steps:
//  1. Validate input shape (with the configured cap this time)
//  2. Resolve to an absolute path
//  3. Reject binary extensions and device paths (defensive)
//  4. Stat the target → branch on exists/notexists:
//     - new file: no read-before-write check
//     - existing file: directory rejection, then read-before-write
//     check via filestate (entry must exist, must be a read entry,
//     on-disk mtime must not have advanced)
//  5. Normalize content (CRLF → LF)
//  6. MkdirAll parent
//  7. WriteFile
//  8. Re-stat for fresh mtime
//  9. Record in filestate as a write entry (Offset/Limit nil)
//  10. Build the WriteOutput variant and return
func runWrite(ctx context.Context, in WriteInput, resolver PathResolver) (WriteOutput, error) {
	cfg := configFromContext(ctx)

	if err := Validate(in, cfg.MaxBytes); err != nil {
		return WriteOutput{}, err
	}
	resolved, err := Resolve(in, resolver)
	if err != nil {
		return WriteOutput{}, err
	}
	absPath := resolved.AbsPath

	// Defensive blocklists.
	if IsBinaryExtension(absPath) {
		return WriteOutput{}, fmt.Errorf("%w: %s", ErrBinaryRejected, absPath)
	}
	if IsBlockedDevicePath(absPath) {
		return WriteOutput{}, fmt.Errorf("%w: %s", ErrDevicePathRejected, absPath)
	}
	if secrets.IsSecretPath(absPath) {
		return WriteOutput{}, secrets.Refusal("file-write", absPath)
	}

	// Stat → branch on exists/notexists.
	info, statErr := os.Stat(absPath)
	isCreate := false
	switch {
	case errors.Is(statErr, fs.ErrNotExist):
		// New file path: no prior read required (FR-FW-05).
		isCreate = true

	case statErr != nil:
		return WriteOutput{}, fmt.Errorf("file-write: stat %s: %w", absPath, statErr)

	case info.IsDir():
		return WriteOutput{}, fmt.Errorf("file-write: %s is a directory", absPath)

	default:
		// Existing file path: read-before-write check via filestate.
		store := filestateFrom(ctx)
		if store == nil {
			return WriteOutput{}, fmt.Errorf("file-write: filestate cache not registered (cannot enforce read-before-write); this is a wiring bug")
		}
		entry, ok := store.Lookup(absPath)
		if !ok || !entry.IsReadEntry() {
			return WriteOutput{}, fmt.Errorf("file-write: %s already exists and must be read first via file-read before overwriting. For new files, use a path that does not exist yet", absPath)
		}
		if info.ModTime().After(entry.Timestamp) {
			return WriteOutput{}, fmt.Errorf("file-write: %s changed on disk since last read; re-read the file first (cached mtime: %s, current mtime: %s)", absPath, entry.Timestamp, info.ModTime())
		}
	}

	// Normalize line endings to LF (FR-FW-08).
	content := strings.ReplaceAll(in.Content, "\r\n", "\n")

	// Create parent directories if needed.
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return WriteOutput{}, fmt.Errorf("file-write: create parent dirs for %s: %w", absPath, err)
	}

	// Write the content. UTF-8 only in MVP (encoding preservation
	// is deferred to ms05/tool-file-write-encoding).
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return WriteOutput{}, fmt.Errorf("file-write: write %s: %w", absPath, err)
	}

	// Re-stat for the post-write mtime, then record in filestate
	// as a write entry (Offset/Limit nil per FR-FR-10 — write
	// entries are NOT dedup candidates for file_read).
	newInfo, err := os.Stat(absPath)
	if err != nil {
		// Write succeeded but re-stat failed — return success
		// without updating the cache. The user can re-read to
		// recover the cache state.
		newInfo = nil
	}
	if newInfo != nil {
		if store := filestateFrom(ctx); store != nil {
			store.Record(absPath, filestate.Entry{
				Content:   content,
				Timestamp: newInfo.ModTime(),
				// Offset and Limit deliberately nil.
			})
		}
	}

	// Build the output variant.
	bytesWritten := len(content)
	var out WriteOutput
	if isCreate {
		out = WriteOutput{
			Type: OutputTypeCreate,
			File: CreateOutputFile{
				FilePath:     absPath,
				BytesWritten: bytesWritten,
			},
		}
	} else {
		out = WriteOutput{
			Type: OutputTypeUpdate,
			File: UpdateOutputFile{
				FilePath:     absPath,
				BytesWritten: bytesWritten,
			},
		}
	}
	out = attachPathHintIfNeeded(out, resolved, resolver)
	return out, nil
}

// attachPathHintIfNeeded adds a hint warning when the LLM used
// absolute_path for a file that turned out to be inside the
// workspace. Better to nudge them now than let them keep using the
// noisier branch.
//
// The hint does NOT fire when absolute_path resolved to a file
// outside the workspace — in that case the user explicitly approved
// the path via the permission gate, and the hint would be noise.
//
// Mirrors file_read's attachPathHintIfNeeded.
func attachPathHintIfNeeded(out WriteOutput, resolved ResolvedPath, resolver PathResolver) WriteOutput {
	if !resolved.FromAbsoluteField {
		return out
	}
	if !ClassifyAgainstWorkspace(resolved.AbsPath, resolver.WorkspaceRoot()) {
		// Outside workspace — user explicitly approved this. No nudge.
		return out
	}
	hint := "this file is inside the workspace; prefer 'workspace_path' next time to skip the permission check"
	if out.Warning == "" {
		out.Warning = hint
	} else {
		out.Warning = out.Warning + "; " + hint
	}
	return out
}

// describeWrite produces a one-line summary of a file_write
// invocation for permission dialogs and progress UI.
func describeWrite(raw json.RawMessage) string {
	var in WriteInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "file-write <invalid input>"
	}
	var b strings.Builder
	b.WriteString("file-write ")
	switch {
	case in.WorkspacePath != "":
		fmt.Fprintf(&b, "workspace_path=%s", in.WorkspacePath)
	case in.AbsolutePath != "":
		fmt.Fprintf(&b, "absolute_path=%s", in.AbsolutePath)
	default:
		b.WriteString("<no path>")
	}
	fmt.Fprintf(&b, " (%d bytes)", len(in.Content))
	return b.String()
}

// filestateFrom is the indirection used by the handler to look up
// the per-session filestate Store. In production it points to
// filestate.From; tests override it via init() in *_test.go to
// inject a test-only store without standing up the full Application
// stack.
//
// **Note**: file_write is fundamentally dependent on filestate for
// the read-before-write check on existing files. file_read's nil-
// tolerance behavior (return file content even with no cache) does
// NOT apply here — the existing-file path actively errors if the
// store is missing in production. Tests inject via the test seam
// below.
var filestateFrom = filestate.From

// configFromContext resolves *FileWriteConfig from the agent
// context. Falls back to DefaultConfig() if no application is wired
// in (test contexts running the handler directly).
//
// Tests can also stuff a config into context via withTestConfig
// (defined in file_write_test.go); the test-shimmed lookup checks
// that key first.
func configFromContext(ctx context.Context) *FileWriteConfig {
	if cfg, ok := ctx.Value(testConfigKey{}).(*FileWriteConfig); ok {
		return cfg
	}
	cfg, err := application.UseService[*FileWriteConfig](ctx)
	if err != nil || cfg == nil {
		return DefaultConfig()
	}
	return cfg
}

// testConfigKey is the context key used by withTestConfig in tests
// to inject a *FileWriteConfig without going through DI. Lives in
// the production file (rather than _test.go) so configFromContext
// can reference it without an internal-only build tag.
type testConfigKey struct{}

// toolDescription is the human-readable text the LLM sees in its
// tool list. Adapted from spec §7 with the path API divergence
// noted up-front so the LLM picks the right field on the first call.
const toolDescription = `Writes a file to the local filesystem.

Path API:
- Use 'workspace_path' for files inside the project (no friction, fast path).
- Use 'absolute_path' only when you need a file outside the project. This will require user approval.
- Exactly one of the two MUST be set per call.

Behavior:
- For NEW files (no prior version on disk), no prior read is required. The tool creates parent directories automatically.
- For EXISTING files, you MUST first read the file via file-read in the current session. Writes without a prior read are rejected.
- If a file has been modified on disk since you last read it, the write is rejected with a 'file changed on disk' error. Re-read the file first.
- Content with CRLF line endings is normalized to LF before writing.
- Always writes UTF-8.

Limits:
- Content over 1 MiB is rejected. For very large files, write in multiple smaller chunks.

Restrictions (MVP scope):
- Binary files (executables, archives, images, PDFs, notebooks) are rejected. file-write is for text content only.
- Device paths (/dev/zero, NUL, etc.) are rejected.
- The output does not yet include a structured diff. Re-read the file via file-read to verify the new content.
`
