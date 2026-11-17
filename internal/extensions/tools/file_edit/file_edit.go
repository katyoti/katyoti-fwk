// SPDX-License-Identifier: Apache-2.0

package file_edit

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
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/pathtool"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

func NewFileEditTool(resolver PathResolver) toolsapi.Tool {
	handler := func(ctx context.Context, in EditInput) (EditOutput, error) {
		return runEdit(ctx, in, resolver)
	}
	return toolsapi.NewFunctionTool(
		"file-edit",
		toolDescription,
		handler,
		toolsapi.WithMutating(),
		toolsapi.WithPermissionCheck(makePermissionFunc(resolver)),
		toolsapi.WithDescriber(describeEdit),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "✏️", Label: "Edit", Color: "#FFCB6B", Template: "{{.file_path}}"}),
	)
}

func runEdit(ctx context.Context, in EditInput, resolver PathResolver) (EditOutput, error) {
	if in.OldString == in.NewString {
		return EditOutput{}, fmt.Errorf("file-edit: old_string and new_string are identical — no change to make")
	}
	if err := validatePaths(in); err != nil {
		return EditOutput{}, err
	}

	resolved, err := pathtool.Resolve(in.WorkspacePath, in.AbsolutePath, resolver, "file-edit")
	if err != nil {
		return EditOutput{}, err
	}
	absPath := resolved.AbsPath

	// .ipynb check BEFORE the generic binary blocklist so the error
	// message specifically directs the user to the notebook edit tool
	// (FR-FE-13). .ipynb is also in the binary blocklist but the
	// generic "binary not supported" message is less helpful.
	if strings.ToLower(filepath.Ext(absPath)) == ".ipynb" {
		return EditOutput{}, fmt.Errorf("file-edit: .ipynb files cannot be edited with file-edit; use the notebook edit tool instead")
	}
	if pathtool.IsBinaryExtension(absPath) {
		return EditOutput{}, fmt.Errorf("file-edit: binary file types not supported: %s", absPath)
	}
	if pathtool.IsBlockedDevicePath(absPath) {
		return EditOutput{}, fmt.Errorf("file-edit: device paths blocked: %s", absPath)
	}
	if secrets.IsSecretPath(absPath) {
		return EditOutput{}, secrets.Refusal("file-edit", absPath)
	}

	info, statErr := os.Stat(absPath)
	switch {
	case errors.Is(statErr, fs.ErrNotExist):
		if in.OldString != "" {
			return EditOutput{}, fmt.Errorf("file-edit: file %s does not exist (and old_string is non-empty; use file-write to create files)", absPath)
		}
		content := in.NewString
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return EditOutput{}, fmt.Errorf("file-edit: create parent dirs: %w", err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			return EditOutput{}, fmt.Errorf("file-edit: create file: %w", err)
		}
		if store := filestateFrom(ctx); store != nil {
			newInfo, _ := os.Stat(absPath)
			if newInfo != nil {
				store.Record(absPath, filestate.Entry{Content: content, Timestamp: newInfo.ModTime()})
			}
		}
		out := EditOutput{FilePath: absPath, Replacements: 1, Created: true}
		out.Warning = pathtool.AttachPathHintIfNeeded(out.Warning, resolved, resolver)
		return out, nil

	case statErr != nil:
		return EditOutput{}, fmt.Errorf("file-edit: stat %s: %w", absPath, statErr)

	case info.IsDir():
		return EditOutput{}, fmt.Errorf("file-edit: %s is a directory", absPath)
	}

	store := filestateFrom(ctx)
	if store == nil {
		return EditOutput{}, fmt.Errorf("file-edit: filestate cache not registered")
	}
	entry, ok := store.Lookup(absPath)
	if !ok || !entry.IsReadEntry() {
		return EditOutput{}, fmt.Errorf("file-edit: %s must be read first via file-read before it can be edited", absPath)
	}
	if info.ModTime().After(entry.Timestamp) {
		return EditOutput{}, fmt.Errorf("file-edit: %s changed on disk since last read; re-read first", absPath)
	}

	if in.OldString == "" && info.Size() > 0 {
		return EditOutput{}, fmt.Errorf("file-edit: cannot use empty old_string on an existing non-empty file; use file-write for full overwrites")
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return EditOutput{}, fmt.Errorf("file-edit: read %s: %w", absPath, err)
	}
	content := string(data)

	count := strings.Count(content, in.OldString)
	if count == 0 {
		return EditOutput{}, fmt.Errorf("file-edit: old_string not found in %s", absPath)
	}
	if !in.ReplaceAll && count > 1 {
		return EditOutput{}, fmt.Errorf("file-edit: old_string appears %d times in %s (must be unique, or use replace_all=true)", count, absPath)
	}

	var newContent string
	replacements := 0
	if in.ReplaceAll {
		newContent = strings.ReplaceAll(content, in.OldString, in.NewString)
		replacements = count
	} else {
		newContent = strings.Replace(content, in.OldString, in.NewString, 1)
		replacements = 1
	}

	if err := os.WriteFile(absPath, []byte(newContent), 0o644); err != nil {
		return EditOutput{}, fmt.Errorf("file-edit: write %s: %w", absPath, err)
	}

	newInfo, _ := os.Stat(absPath)
	if newInfo != nil && store != nil {
		store.Record(absPath, filestate.Entry{Content: newContent, Timestamp: newInfo.ModTime()})
	}

	out := EditOutput{FilePath: absPath, Replacements: replacements}
	out.Warning = pathtool.AttachPathHintIfNeeded(out.Warning, resolved, resolver)
	return out, nil
}

func describeEdit(raw json.RawMessage) string {
	var in EditInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "file-edit <invalid input>"
	}
	var b strings.Builder
	b.WriteString("file-edit ")
	switch {
	case in.WorkspacePath != "":
		fmt.Fprintf(&b, "workspace_path=%s", in.WorkspacePath)
	case in.AbsolutePath != "":
		fmt.Fprintf(&b, "absolute_path=%s", in.AbsolutePath)
	default:
		b.WriteString("<no path>")
	}
	if in.ReplaceAll {
		b.WriteString(" replace_all=true")
	}
	return b.String()
}

var filestateFrom = filestate.From

func configFromContext(ctx context.Context) *FileEditConfig {
	cfg, err := application.UseService[*FileEditConfig](ctx)
	if err != nil || cfg == nil {
		return DefaultConfig()
	}
	return cfg
}

type FileEditConfig struct {
	MaxEditableBytes int
}

func DefaultConfig() *FileEditConfig {
	return &FileEditConfig{MaxEditableBytes: 1 << 30}
}

type testConfigKey struct{}

const toolDescription = `Performs exact string replacements in files.

Path API:
- Use 'workspace_path' for files inside the project (no friction).
- Use 'absolute_path' only when the file is outside the project (requires user approval).
- Exactly one of the two MUST be set per call.

Behavior:
- You must read the file via file-read first. Edits without a prior read are rejected.
- old_string must differ from new_string.
- By default, old_string must appear exactly once. If it appears multiple times, provide more context or set replace_all=true.
- Use replace_all=true for bulk renames across the file.
- An empty old_string on a non-existent file creates the file with new_string as content.

Restrictions:
- Binary files and .ipynb files are rejected.
- Device paths (/dev/zero, NUL, etc.) are rejected.
`
