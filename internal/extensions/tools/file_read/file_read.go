// SPDX-License-Identifier: Apache-2.0

// Package file_read implements the file_read agent tool.
//
// Spec: docs/agent_tools/tool_file_read_spec.md
//
// MVP scope (ms04-p1): text reading with line numbers, partial reads,
// size and token caps, deduplication via filestate.Store, hybrid path
// API (workspace_path XOR absolute_path), workspace boundary check on
// the absolute branch (out-of-workspace reads require user approval),
// binary and device blocklists. Image, PDF, and notebook variants are
// reserved in schema.go but deferred to ms05/tool-file-read-rich-formats.
//
// File layout:
//   - schema.go      — input/output types, ReadOutputType constants,
//     PathResolver interface
//   - config.go      — FileReadConfig and DI extension
//   - validate.go    — validation, path resolution, blocklists,
//     workspace boundary check
//   - permission.go  — CheckPermissions policy
//   - file_read.go   — tool constructor and handler (this file)
//   - *_test.go      — unit tests
package file_read

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/filestate"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

// NewFileReadTool creates a file_read tool wired to the given path
// resolver. The resolver MUST satisfy PathResolver (see schema.go) —
// it must expose both Resolve and WorkspaceRoot.
//
// The tool's config and per-session filestate Store are resolved per
// call from context, NOT captured at construction. This lets:
//   - tests override the config without rebuilding the tool
//   - per-session filestate stores work correctly
//   - the same tool instance serve multiple sessions safely
//
// The PathResolver IS captured because it represents the workspace
// boundary, which is a tool-construction-time concern (one workspace
// per tool instance).
func NewFileReadTool(resolver PathResolver) toolsapi.Tool {
	handler := func(ctx context.Context, in ReadInput) (ReadOutput, error) {
		return runRead(ctx, in, resolver)
	}
	return toolsapi.NewFunctionTool(
		"file-read",
		toolDescription,
		handler,
		toolsapi.WithReadOnly(),
		toolsapi.WithPermissionCheck(makePermissionFunc(resolver)),
		toolsapi.WithDescriber(describeRead),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📄", Label: "Read", Color: "#82AAFF", Template: "{{.file_path}}"}),
	)
}

// describeRead produces a one-line summary of a file_read invocation
// for permission dialogs and progress UI. Output examples:
//
//	file_read workspace_path=foo/bar.go
//	file_read absolute_path=/etc/hosts (outside workspace)
//	file_read workspace_path=big.log offset=1000 limit=200
func describeRead(raw json.RawMessage) string {
	var in ReadInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "file-read <invalid input>"
	}
	var b strings.Builder
	b.WriteString("file-read ")
	switch {
	case in.WorkspacePath != "":
		fmt.Fprintf(&b, "workspace_path=%s", in.WorkspacePath)
	case in.AbsolutePath != "":
		fmt.Fprintf(&b, "absolute_path=%s", in.AbsolutePath)
	default:
		b.WriteString("<no path>")
	}
	if in.Offset != nil {
		fmt.Fprintf(&b, " offset=%d", *in.Offset)
	}
	if in.Limit != nil {
		fmt.Fprintf(&b, " limit=%d", *in.Limit)
	}
	return b.String()
}

// runRead is the full handler body. It re-runs Validate and Resolve
// (the permission gate already did so, but the handler must not trust
// the gate's prior work — the gate result is not threaded through to
// the handler in the current kernel design, and re-running is cheap).
//
// Steps:
//  1. Validate input shape
//  2. Resolve to an absolute path
//  3. Reject binary extensions and device paths (defensive)
//  4. Stat the file → enforce size cap
//  5. Read the requested window (full or offset+limit)
//  6. Format with cat-n line numbers
//  7. Enforce token cap
//  8. Check filestate dedup → return file_unchanged if hit
//  9. Record the read in filestate
//  10. Build the TextOutput variant and return
func runRead(ctx context.Context, in ReadInput, resolver PathResolver) (ReadOutput, error) {
	if err := Validate(in); err != nil {
		return ReadOutput{}, err
	}
	resolved, err := Resolve(in, resolver)
	if err != nil {
		return ReadOutput{}, err
	}
	absPath := resolved.AbsPath

	// Defensive blocklists (subtask 6).
	if IsBinaryExtension(absPath) {
		return ReadOutput{}, fmt.Errorf("%w: %s", ErrBinaryRejected, absPath)
	}
	if IsBlockedDevicePath(absPath) {
		return ReadOutput{}, fmt.Errorf("%w: %s", ErrDevicePathRejected, absPath)
	}
	if secrets.IsSecretPath(absPath) {
		return ReadOutput{}, secrets.Refusal("file-read", absPath)
	}

	// Resolve config from context (tests can override; production uses
	// the registered FileReadConfigExtension default).
	cfg := configFromContext(ctx)

	// Stat → size cap.
	info, err := os.Stat(absPath)
	if err != nil {
		return ReadOutput{}, fmt.Errorf("file-read: stat %s: %w", absPath, err)
	}
	if info.IsDir() {
		return ReadOutput{}, fmt.Errorf("file-read: %s is a directory; use glob or grep instead", absPath)
	}
	if info.Size() > int64(cfg.MaxBytes) {
		return ReadOutput{}, fmt.Errorf(
			"file-read: %s is %d bytes which exceeds the %d-byte cap; use offset and limit to read a slice",
			absPath, info.Size(), cfg.MaxBytes,
		)
	}

	// Determine the read window.
	offset := 0
	if in.Offset != nil {
		offset = *in.Offset
	}
	limit := cfg.DefaultLines
	if in.Limit != nil {
		limit = *in.Limit
	}

	// Dedup check BEFORE reading. If we have a matching cache entry
	// and the file's mtime is unchanged, short-circuit. We compare
	// against the on-disk mtime from the Stat above, which is the
	// authoritative current state.
	mtime := info.ModTime()
	store := filestateFrom(ctx)
	if store != nil {
		if entry, ok := store.Lookup(absPath); ok && entry.IsReadEntry() {
			if entry.Timestamp.Equal(mtime) &&
				entry.Offset != nil && *entry.Offset == offset &&
				entry.Limit != nil && *entry.Limit == limit {
				// Dedup hit: return the stub WITHOUT updating the
				// cache (FR-FR-09: original entry preserved).
				return ReadOutput{
					Type: OutputTypeFileUnchanged,
					File: FileUnchangedOutputFile{FilePath: absPath},
				}, nil
			}
		}
	}

	// Read the requested window. We always need totalLines for the
	// output metadata, so we scan the entire file once and slice the
	// window in memory. The size cap above bounds memory.
	allLines, err := readAllLines(absPath)
	if err != nil {
		return ReadOutput{}, err
	}
	totalLines := len(allLines)

	// Empty file → warning, not error (FR-FR-26).
	if totalLines == 0 {
		out := ReadOutput{
			Type: OutputTypeText,
			File: TextOutputFile{
				FilePath:   absPath,
				Content:    "",
				NumLines:   0,
				StartLine:  1,
				TotalLines: 0,
			},
			Warning: "file is empty",
		}
		recordRead(store, absPath, "", mtime, offset, limit, false)
		out = attachPathHintIfNeeded(out, resolved, resolver)
		return out, nil
	}

	// Offset past EOF → warning, not error (FR-FR-27).
	if offset >= totalLines {
		out := ReadOutput{
			Type: OutputTypeText,
			File: TextOutputFile{
				FilePath:   absPath,
				Content:    "",
				NumLines:   0,
				StartLine:  offset + 1,
				TotalLines: totalLines,
			},
			Warning: fmt.Sprintf("offset %d is past end of file (file has %d lines)", offset, totalLines),
		}
		recordRead(store, absPath, "", mtime, offset, limit, false)
		out = attachPathHintIfNeeded(out, resolved, resolver)
		return out, nil
	}

	// Slice the window.
	end := min(offset+limit, totalLines)
	window := allLines[offset:end]
	isPartial := offset > 0 || end < totalLines

	// Format with cat-n line numbers (line, tab, content).
	formatted := formatLines(window, offset+1)

	// Token cap check on the formatted output.
	if est := cfg.EstimateTokens(formatted); est > cfg.MaxTokens {
		return ReadOutput{}, fmt.Errorf(
			"file-read: estimated %d tokens exceeds the %d-token cap; use offset and limit to read a smaller slice",
			est, cfg.MaxTokens,
		)
	}

	// Record in filestate (FR-FR-08).
	recordRead(store, absPath, formatted, mtime, offset, limit, isPartial)

	out := ReadOutput{
		Type: OutputTypeText,
		File: TextOutputFile{
			FilePath:   absPath,
			Content:    formatted,
			NumLines:   len(window),
			StartLine:  offset + 1,
			TotalLines: totalLines,
		},
	}
	out = attachPathHintIfNeeded(out, resolved, resolver)
	return out, nil
}

// readAllLines reads the entire file into memory, splitting on
// newlines. The size cap is enforced upstream by the caller via
// os.Stat, so the in-memory cost is bounded.
//
// We use bufio.Scanner with a generous max token size (1 MB) so a
// single very long line does not blow up. The scanner strips trailing
// newlines from each line, which is what we want for cat-n
// formatting.
func readAllLines(absPath string) ([]string, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("file-read: open %s: %w", absPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// 1 MB per line is enough for any source file we care about and
	// stops a single pathological line from exhausting memory.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("file-read: scan %s: %w", absPath, err)
	}
	return lines, nil
}

// formatLines renders a slice of lines in cat -n format starting
// from the given 1-based line number.
//
// Format: line number right-padded with spaces to width 6, tab, line
// content. Width 6 is enough for files up to 999999 lines and matches
// what `cat -n` produces by default.
func formatLines(lines []string, firstLineNumber int) string {
	var b strings.Builder
	// Pre-size: each line is roughly its content + ~10 chars overhead.
	totalLen := 0
	for _, l := range lines {
		totalLen += len(l) + 10
	}
	b.Grow(totalLen)

	for i, line := range lines {
		fmt.Fprintf(&b, "%6d\t%s\n", firstLineNumber+i, line)
	}
	return b.String()
}

// recordRead writes an entry to the filestate store. Safe to call
// with a nil store (test contexts without an Application registered).
func recordRead(store filestate.Store, absPath, content string, mtime time.Time, offset, limit int, isPartial bool) {
	if store == nil {
		return
	}
	o, l := offset, limit
	store.Record(absPath, filestate.Entry{
		Content:       content,
		Timestamp:     mtime,
		Offset:        &o,
		Limit:         &l,
		IsPartialView: isPartial,
	})
}

// attachPathHintIfNeeded adds a hint warning when the LLM used
// absolute_path for a file that turned out to be inside the
// workspace. Better to nudge them now than let them keep using the
// noisier branch.
//
// The hint does NOT fire when absolute_path resolved to a file
// outside the workspace — in that case the user explicitly approved
// the read via the permission gate, and the hint would be noise.
//
// We don't overwrite an existing Warning (e.g. "file is empty") —
// we append, separated by "; ".
func attachPathHintIfNeeded(out ReadOutput, resolved ResolvedPath, resolver PathResolver) ReadOutput {
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

// filestateFrom is the indirection used by the handler to look up
// the per-session filestate Store. In production it points to
// filestate.From; tests can override it via init() in *_test.go to
// inject a test-only store without standing up the full Application
// stack.
var filestateFrom = filestate.From

// configFromContext resolves *FileReadConfig from the agent context.
// Falls back to DefaultConfig() if no application is wired in (test
// contexts running the handler directly).
//
// Tests can also stuff a config into context via withTestConfig
// (defined in file_read_test.go); the test-shimmed lookup checks
// that key first.
func configFromContext(ctx context.Context) *FileReadConfig {
	if cfg, ok := ctx.Value(testConfigKey{}).(*FileReadConfig); ok {
		return cfg
	}
	cfg, err := application.UseService[*FileReadConfig](ctx)
	if err != nil || cfg == nil {
		return DefaultConfig()
	}
	return cfg
}

// testConfigKey is the context key used by withTestConfig in tests
// to inject a *FileReadConfig without going through DI. Lives in the
// production file (rather than _test.go) so configFromContext can
// reference it without an internal-only build tag.
type testConfigKey struct{}

// toolDescription is the human-readable text the LLM sees in its tool
// list. Adapted from spec §7 with the path API divergence noted
// up-front so the LLM picks the right field on the first call.
const toolDescription = `Reads a file from the filesystem and returns its content with line numbers.

Path API:
- Use 'workspace_path' for files inside the project (no friction, fast path).
- Use 'absolute_path' only when you need a file outside the project. This will require user approval.
- Exactly one of the two MUST be set per call.

Output:
- Text content is returned with cat -n formatting (line number, tab, content).
- By default the tool reads up to 2000 lines from the start of the file.
- Use 'offset' (0-based) and 'limit' to read a specific range when the file is large.
- If the same file with the same offset+limit was already read in this session and has not been modified on disk since, the tool returns a 'file_unchanged' stub instead of re-sending the content.

Limits:
- Files larger than 256 KB are rejected — use offset and limit to read a slice.
- Output exceeding ~25000 estimated tokens is rejected — use offset and limit to read a slice.
- Empty files return a warning, not an error.

Restrictions (MVP scope):
- Binary files (executables, archives, images, PDFs, notebooks) are rejected. Image/PDF/notebook reading lands in a follow-up tool.
- Device paths (/dev/zero, NUL, etc.) are rejected.
`
