// SPDX-License-Identifier: Apache-2.0

// Package glob implements the glob tool for file pattern matching.
package glob

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/pathtool"
)

const defaultMaxResults = 100

type PathResolver = pathtool.PathResolver

type GlobInput struct {
	Pattern       string `json:"pattern"                jsonschema:"The glob pattern to match files against" required:"true"`
	WorkspacePath string `json:"workspace_path,omitempty" jsonschema:"Workspace-relative directory to search in. Defaults to workspace root. Mutually exclusive with absolute_path."`
	AbsolutePath  string `json:"absolute_path,omitempty"  jsonschema:"Absolute filesystem path to search. Requires approval if outside workspace. Mutually exclusive with workspace_path."`
}

type GlobOutput struct {
	Filenames  []string `json:"filenames"`
	DurationMs int64    `json:"duration_ms"`
	NumFiles   int      `json:"num_files"`
	Truncated  bool     `json:"truncated"`
	Message    string   `json:"message,omitempty"`
	Warning    string   `json:"warning,omitempty"`
}

type fileEntry struct {
	relPath string
	modTime time.Time
}

// NewGlobTool creates a glob tool. Pass a resolver for the workspace
// path API and permission policy; pass nothing for backward compat
// (uses CWD, no permission check).
func NewGlobTool(resolvers ...PathResolver) toolsapi.Tool {
	var resolver PathResolver
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	}
	handler := func(ctx context.Context, in GlobInput) (GlobOutput, error) {
		return executeGlob(ctx, in, resolver)
	}
	opts := []toolsapi.ToolOption{toolsapi.WithReadOnly()}
	if resolver != nil {
		opts = append(opts, toolsapi.WithPermissionCheck(makePermissionFunc(resolver)))
	}
	opts = append(opts, toolsapi.WithDescriber(describeGlob))
	opts = append(opts, toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "📂", Label: "Glob", Color: "#89DDFF", Template: "{{.pattern}}"}))
	return toolsapi.NewFunctionTool("glob", toolDescription, handler, opts...)
}

func executeGlob(_ context.Context, params GlobInput, resolver PathResolver) (GlobOutput, error) {
	if params.Pattern == "" {
		return GlobOutput{}, fmt.Errorf("glob: pattern is required")
	}
	start := time.Now()

	searchRoot, resolved, err := resolveSearchRoot(params, resolver)
	if err != nil {
		return GlobOutput{}, err
	}
	absRoot, err := filepath.Abs(searchRoot)
	if err != nil {
		return GlobOutput{}, fmt.Errorf("glob: resolve search root: %w", err)
	}
	if isUNCPath(absRoot) {
		return GlobOutput{}, fmt.Errorf("glob: UNC paths are not supported")
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return GlobOutput{}, fmt.Errorf("glob: search path does not exist: %s", absRoot)
		}
		return GlobOutput{}, fmt.Errorf("glob: stat: %w", err)
	}
	if !info.IsDir() {
		return GlobOutput{}, fmt.Errorf("glob: search path is not a directory: %s", absRoot)
	}

	fsys := os.DirFS(absRoot)
	matches, err := doublestar.Glob(fsys, params.Pattern)
	if err != nil {
		return GlobOutput{}, fmt.Errorf("glob: %w", err)
	}

	var entries []fileEntry
	for _, m := range matches {
		absPath := filepath.Join(absRoot, m)
		if isUNCPath(absPath) {
			continue
		}
		finfo, ferr := os.Stat(absPath)
		if ferr != nil || finfo.IsDir() {
			continue
		}
		entries = append(entries, fileEntry{relPath: filepath.ToSlash(m), modTime: finfo.ModTime()})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].modTime.After(entries[j].modTime) })

	truncated := len(entries) > defaultMaxResults
	if truncated {
		entries = entries[:defaultMaxResults]
	}

	filenames := make([]string, len(entries))
	for i, e := range entries {
		filenames[i] = e.relPath
	}

	out := GlobOutput{
		Filenames: filenames, DurationMs: time.Since(start).Milliseconds(),
		NumFiles: len(filenames), Truncated: truncated,
	}
	if truncated {
		out.Message = "Results truncated. Use a more specific pattern or path."
	}
	if len(filenames) == 0 {
		out.Filenames = []string{}
		out.Message = "No files found"
	}
	if resolved != nil && resolver != nil {
		out.Warning = pathtool.AttachPathHintIfNeeded("", *resolved, resolver)
	}
	return out, nil
}

func resolveSearchRoot(params GlobInput, resolver PathResolver) (string, *pathtool.ResolvedPath, error) {
	hasWS := params.WorkspacePath != ""
	hasAbs := params.AbsolutePath != ""
	if hasWS && hasAbs {
		return "", nil, fmt.Errorf("glob: set exactly one of workspace_path or absolute_path, not both")
	}
	if !hasWS && !hasAbs {
		if resolver != nil {
			return resolver.WorkspaceRoot(), nil, nil
		}
		return ".", nil, nil
	}
	if resolver == nil {
		if hasWS {
			return params.WorkspacePath, nil, nil
		}
		exp, err := pathtool.ExpandTilde(params.AbsolutePath)
		return exp, nil, err
	}
	resolved, err := pathtool.Resolve(params.WorkspacePath, params.AbsolutePath, resolver, "glob")
	if err != nil {
		return "", nil, err
	}
	return resolved.AbsPath, &resolved, nil
}

func makePermissionFunc(resolver PathResolver) toolsapi.PermissionFunc {
	return func(_ context.Context, raw json.RawMessage) toolsapi.PermissionResult {
		var in GlobInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionDeny, Message: fmt.Sprintf("glob: malformed input: %v", err)}
		}
		if in.WorkspacePath == "" && in.AbsolutePath == "" {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}
		}
		if in.WorkspacePath != "" && in.AbsolutePath != "" {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionDeny, Message: "glob: set exactly one of workspace_path or absolute_path"}
		}
		resolved, err := pathtool.Resolve(in.WorkspacePath, in.AbsolutePath, resolver, "glob")
		if err != nil {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionDeny, Message: err.Error()}
		}
		if !resolved.FromAbsoluteField {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}
		}
		if pathtool.ClassifyAgainstWorkspace(resolved.AbsPath, resolver.WorkspaceRoot()) {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}
		}
		slashAbs := filepath.ToSlash(resolved.AbsPath)
		return toolsapi.PermissionResult{
			Behavior:    toolsapi.PermissionAsk,
			Message:     fmt.Sprintf("glob wants to search %q, outside workspace %q. Allow?", resolved.AbsPath, resolver.WorkspaceRoot()),
			Dimensions:  map[string]string{"absolute_path": slashAbs},
			Suggestions: pathtool.BuildOutsideWorkspaceSuggestions("glob", slashAbs),
		}
	}
}

func describeGlob(raw json.RawMessage) string {
	var in GlobInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "glob <invalid>"
	}
	var b strings.Builder
	b.WriteString("glob pattern=" + in.Pattern)
	if in.WorkspacePath != "" {
		b.WriteString(" workspace_path=" + in.WorkspacePath)
	} else if in.AbsolutePath != "" {
		b.WriteString(" absolute_path=" + in.AbsolutePath)
	}
	return b.String()
}

func isUNCPath(p string) bool {
	return strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//")
}

const toolDescription = `Fast file pattern matching tool that works with any codebase size.
Supports glob patterns like "**/*.js" or "src/**/*.ts".
Returns matching file paths sorted by modification time.

Path API:
- Omit both workspace_path and absolute_path to search from the workspace root (default).
- Use 'workspace_path' for a subdirectory inside the project.
- Use 'absolute_path' only when searching outside the workspace (requires user approval).
`
