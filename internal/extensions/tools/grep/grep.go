// SPDX-License-Identifier: Apache-2.0

// Package grep implements the grep agent tool for regex content search.
//
// Spec: docs/agent_tools/tool_grep_spec.md
// MVP scope: regex search across files, files_with_matches + content +
// count output modes, context lines, head_limit + offset pagination,
// file-type and glob filtering, case-insensitive search, multiline mode.
// Uses Go stdlib regexp + filepath.Walk. No external ripgrep dependency.
package grep

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/pathtool"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

type PathResolver = pathtool.PathResolver

// GrepInput is the input schema.
type GrepInput struct {
	Pattern       string `json:"pattern"                   jsonschema:"Regex pattern to search for" required:"true"`
	WorkspacePath string `json:"workspace_path,omitempty"   jsonschema:"Workspace-relative path to search. Mutually exclusive with absolute_path."`
	AbsolutePath  string `json:"absolute_path,omitempty"    jsonschema:"Absolute path to search. Mutually exclusive with workspace_path."`
	Glob          string `json:"glob,omitempty"             jsonschema:"Glob pattern to filter files (e.g. '*.js', '*.{ts,tsx}')"`
	Type          string `json:"type,omitempty"             jsonschema:"File type filter (e.g. 'js', 'py', 'go')"`
	OutputMode    string `json:"output_mode,omitempty"      jsonschema:"Output mode: 'files_with_matches' (default), 'content', 'count'"`
	ContextBefore *int   `json:"context_before,omitempty"   jsonschema:"Lines before each match (-B). Content mode only."`
	ContextAfter  *int   `json:"context_after,omitempty"    jsonschema:"Lines after each match (-A). Content mode only."`
	Context       *int   `json:"context,omitempty"          jsonschema:"Symmetric context lines (-C). Content mode only."`
	HeadLimit     *int   `json:"head_limit,omitempty"       jsonschema:"Max entries to return. 0=unlimited. Default 250."`
	Offset        int    `json:"offset,omitempty"           jsonschema:"Skip first N entries before applying head_limit."`
	CaseInsens    bool   `json:"case_insensitive,omitempty" jsonschema:"Case-insensitive search."`
	Multiline     bool   `json:"multiline,omitempty"        jsonschema:"Enable multiline mode (. matches newlines)."`
	LineNumbers   *bool  `json:"line_numbers,omitempty"     jsonschema:"Show line numbers in content mode. Default true."`
}

// GrepOutput is the output.
type GrepOutput struct {
	Content   string `json:"content,omitempty"           jsonschema:"Matching content (content mode) or file list or counts."`
	FileCount int    `json:"file_count"                  jsonschema:"Number of files with matches."`
	Truncated bool   `json:"truncated,omitempty"         jsonschema:"True when head_limit truncated results."`
	Message   string `json:"message,omitempty"`
	Warning   string `json:"warning,omitempty"`
}

const defaultHeadLimit = 250
const maxLineLen = 500

// vcsExcludes is the set of version-control directories to skip.
var vcsExcludes = map[string]bool{
	".git": true, ".svn": true, ".hg": true, ".bzr": true, ".jj": true, ".sl": true,
}

// typeExtMap maps file type shortnames to extensions.
var typeExtMap = map[string]string{
	"go": ".go", "js": ".js", "ts": ".ts", "tsx": ".tsx", "py": ".py",
	"rs": ".rs", "java": ".java", "c": ".c", "cpp": ".cpp", "h": ".h",
	"yaml": ".yaml", "yml": ".yml", "json": ".json", "md": ".md",
	"html": ".html", "css": ".css", "sh": ".sh", "rb": ".rb",
	"php": ".php", "swift": ".swift", "kt": ".kt",
}

func NewGrepTool(resolvers ...PathResolver) toolsapi.Tool {
	var resolver PathResolver
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	}
	handler := func(ctx context.Context, in GrepInput) (GrepOutput, error) {
		return executeGrep(ctx, in, resolver)
	}
	opts := []toolsapi.ToolOption{toolsapi.WithReadOnly()}
	if resolver != nil {
		opts = append(opts, toolsapi.WithPermissionCheck(makePermissionFunc(resolver)))
	}
	opts = append(opts, toolsapi.WithDescriber(describeGrep))
	opts = append(opts, toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "🔎", Label: "Grep", Color: "#89DDFF", Template: "{{.pattern}}"}))
	return toolsapi.NewFunctionTool("grep", toolDescription, handler, opts...)
}

func executeGrep(_ context.Context, in GrepInput, resolver PathResolver) (GrepOutput, error) {
	if in.Pattern == "" {
		return GrepOutput{}, fmt.Errorf("grep: pattern is required")
	}

	// Compile the regex.
	flags := ""
	if in.CaseInsens {
		flags += "(?i)"
	}
	if in.Multiline {
		flags += "(?s)"
	}
	re, err := regexp.Compile(flags + in.Pattern)
	if err != nil {
		return GrepOutput{}, fmt.Errorf("grep: invalid regex: %w", err)
	}

	// Resolve search root.
	searchRoot, resolved, err := resolveSearchRoot(in, resolver)
	if err != nil {
		return GrepOutput{}, err
	}
	absRoot, err := filepath.Abs(searchRoot)
	if err != nil {
		return GrepOutput{}, fmt.Errorf("grep: resolve root: %w", err)
	}
	if secrets.IsSecretPath(absRoot) {
		return GrepOutput{}, secrets.Refusal("grep", absRoot)
	}

	// Determine head limit.
	headLimit := defaultHeadLimit
	if in.HeadLimit != nil {
		headLimit = *in.HeadLimit
	}

	mode := in.OutputMode
	if mode == "" {
		mode = "files_with_matches"
	}

	// Walk the directory and search.
	type fileMatch struct {
		relPath string
		modTime time.Time
		lines   []string // content mode
		count   int      // count mode
	}

	var matches []fileMatch
	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip errors gracefully
		}
		if info.IsDir() {
			if vcsExcludes[info.Name()] || secrets.IsSecretPath(path) {
				return filepath.SkipDir
			}
			return nil
		}
		// Never read a secret file's content (.env*, keys, tokens).
		if secrets.IsSecretPath(path) {
			return nil
		}

		// Type filter.
		if in.Type != "" {
			ext, ok := typeExtMap[strings.ToLower(in.Type)]
			if ok && !strings.HasSuffix(strings.ToLower(path), ext) {
				return nil
			}
		}

		// Glob filter.
		if in.Glob != "" {
			relToRoot, _ := filepath.Rel(absRoot, path)
			matched, _ := doublestar.Match(in.Glob, filepath.ToSlash(relToRoot))
			if !matched {
				return nil
			}
		}

		// Skip binary-ish files by extension.
		if pathtool.IsBinaryExtension(path) {
			return nil
		}

		relPath, _ := filepath.Rel(absRoot, path)
		relPath = filepath.ToSlash(relPath)

		switch mode {
		case "files_with_matches":
			if fileContainsMatch(path, re) {
				mt := info.ModTime()
				matches = append(matches, fileMatch{relPath: relPath, modTime: mt})
			}
		case "content":
			lines := searchFileContent(path, re, in)
			if len(lines) > 0 {
				matches = append(matches, fileMatch{relPath: relPath, modTime: info.ModTime(), lines: lines})
			}
		case "count":
			c := countMatches(path, re)
			if c > 0 {
				matches = append(matches, fileMatch{relPath: relPath, count: c})
			}
		}
		return nil
	})
	if err != nil {
		return GrepOutput{}, fmt.Errorf("grep: walk: %w", err)
	}

	// Sort files_with_matches by mtime (newest first), filename tiebreak.
	if mode == "files_with_matches" {
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].modTime.Equal(matches[j].modTime) {
				return matches[i].relPath < matches[j].relPath
			}
			return matches[i].modTime.After(matches[j].modTime)
		})
	}

	// Build output string.
	var out GrepOutput
	out.FileCount = len(matches)

	switch mode {
	case "files_with_matches":
		// Apply offset + head_limit.
		entries := make([]string, 0, len(matches))
		for _, m := range matches {
			entries = append(entries, m.relPath)
		}
		entries, truncated := paginate(entries, in.Offset, headLimit)
		out.Content = strings.Join(entries, "\n")
		out.Truncated = truncated
		if len(entries) == 0 {
			out.Message = "No files found"
		}

	case "content":
		var allLines []string
		for _, m := range matches {
			for _, l := range m.lines {
				allLines = append(allLines, m.relPath+":"+l)
			}
		}
		allLines, truncated := paginate(allLines, in.Offset, headLimit)
		out.Content = strings.Join(allLines, "\n")
		out.Truncated = truncated
		if len(allLines) == 0 {
			out.Message = "No matches found"
		}

	case "count":
		var lines []string
		total := 0
		for _, m := range matches {
			lines = append(lines, fmt.Sprintf("%s:%d", m.relPath, m.count))
			total += m.count
		}
		lines, truncated := paginate(lines, in.Offset, headLimit)
		out.Content = strings.Join(lines, "\n")
		out.Truncated = truncated
		if total == 0 {
			out.Message = "No matches found"
		}
	}

	if resolved != nil && resolver != nil {
		out.Warning = pathtool.AttachPathHintIfNeeded("", *resolved, resolver)
	}
	return out, nil
}

func fileContainsMatch(path string, re *regexp.Regexp) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > maxLineLen {
			line = line[:maxLineLen]
		}
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func searchFileContent(path string, re *regexp.Regexp, in GrepInput) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	before := 0
	after := 0
	if in.Context != nil {
		before = *in.Context
		after = *in.Context
	}
	if in.ContextBefore != nil {
		before = *in.ContextBefore
	}
	if in.ContextAfter != nil {
		after = *in.ContextAfter
	}

	showLineNums := true
	if in.LineNumbers != nil {
		showLineNums = *in.LineNumbers
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var allLines []string
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		allLines = append(allLines, scanner.Text())
	}

	var result []string
	emitted := make(map[int]bool)
	for i, line := range allLines {
		truncLine := line
		if len(truncLine) > maxLineLen {
			truncLine = truncLine[:maxLineLen]
		}
		if !re.MatchString(truncLine) {
			continue
		}
		// Emit context range.
		start := i - before
		if start < 0 {
			start = 0
		}
		end := i + after
		if end >= len(allLines) {
			end = len(allLines) - 1
		}
		for j := start; j <= end; j++ {
			if emitted[j] {
				continue
			}
			emitted[j] = true
			l := allLines[j]
			if len(l) > maxLineLen {
				l = l[:maxLineLen]
			}
			if showLineNums {
				result = append(result, fmt.Sprintf("%d:%s", j+1, l))
			} else {
				result = append(result, l)
			}
		}
	}
	return result
}

func countMatches(path string, re *regexp.Regexp) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > maxLineLen {
			line = line[:maxLineLen]
		}
		matches := re.FindAllStringIndex(line, -1)
		count += len(matches)
	}
	return count
}

func paginate(items []string, offset, limit int) ([]string, bool) {
	if offset > 0 {
		if offset >= len(items) {
			return []string{}, false
		}
		items = items[offset:]
	}
	if limit > 0 && len(items) > limit {
		return items[:limit], true
	}
	return items, false
}

func resolveSearchRoot(in GrepInput, resolver PathResolver) (string, *pathtool.ResolvedPath, error) {
	hasWS := in.WorkspacePath != ""
	hasAbs := in.AbsolutePath != ""
	if hasWS && hasAbs {
		return "", nil, fmt.Errorf("grep: set exactly one of workspace_path or absolute_path, not both")
	}
	if !hasWS && !hasAbs {
		if resolver != nil {
			return resolver.WorkspaceRoot(), nil, nil
		}
		return ".", nil, nil
	}
	if resolver == nil {
		if hasWS {
			return in.WorkspacePath, nil, nil
		}
		exp, err := pathtool.ExpandTilde(in.AbsolutePath)
		return exp, nil, err
	}
	resolved, err := pathtool.Resolve(in.WorkspacePath, in.AbsolutePath, resolver, "grep")
	if err != nil {
		return "", nil, err
	}
	return resolved.AbsPath, &resolved, nil
}

func makePermissionFunc(resolver PathResolver) toolsapi.PermissionFunc {
	return func(_ context.Context, raw json.RawMessage) toolsapi.PermissionResult {
		var in GrepInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionDeny, Message: fmt.Sprintf("grep: malformed input: %v", err)}
		}
		if in.WorkspacePath == "" && in.AbsolutePath == "" {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}
		}
		if in.WorkspacePath != "" && in.AbsolutePath != "" {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionDeny, Message: "grep: set exactly one of workspace_path or absolute_path"}
		}
		resolved, err := pathtool.Resolve(in.WorkspacePath, in.AbsolutePath, resolver, "grep")
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
			Message:     fmt.Sprintf("grep wants to search %q, outside workspace %q. Allow?", resolved.AbsPath, resolver.WorkspaceRoot()),
			Dimensions:  map[string]string{"absolute_path": slashAbs},
			Suggestions: pathtool.BuildOutsideWorkspaceSuggestions("grep", slashAbs),
		}
	}
}

func describeGrep(raw json.RawMessage) string {
	var in GrepInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "grep <invalid>"
	}
	var b strings.Builder
	b.WriteString("grep pattern=" + in.Pattern)
	if in.WorkspacePath != "" {
		b.WriteString(" workspace_path=" + in.WorkspacePath)
	} else if in.AbsolutePath != "" {
		b.WriteString(" absolute_path=" + in.AbsolutePath)
	}
	if in.OutputMode != "" {
		b.WriteString(" mode=" + in.OutputMode)
	}
	return b.String()
}

const toolDescription = `Search file contents using regular expressions.

Path API:
- Omit both workspace_path and absolute_path to search from the workspace root (default).
- Use 'workspace_path' for a subdirectory inside the project.
- Use 'absolute_path' only when searching outside the workspace (requires user approval).

Output modes:
- 'files_with_matches' (default): returns file paths sorted by modification time.
- 'content': returns matching lines with line numbers and optional context.
- 'count': returns per-file match counts.

Features:
- Supports regex patterns, case-insensitive search, multiline mode.
- File type filtering (type='go') and glob filtering (glob='*.{ts,tsx}').
- Context lines: context_before (-B), context_after (-A), context (-C).
- Pagination: head_limit (default 250, 0=unlimited) and offset.
`
