// SPDX-License-Identifier: Apache-2.0

// Package tools registers display decorators for all built-in tools.
// Each decorator defines how the tool's calls and results appear in the
// terminal protocol and TUI. Contributed to DecoratorPoint during bootstrap.
package tools

import (
	"encoding/json"
	"fmt"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

func init() {
	for _, entry := range builtinDecorators() {
		_ = toolsapi.DecoratorPoint.Contribute(entry)
	}
}

// builtinDecorators returns all decorator entries for built-in tools.
func builtinDecorators() []toolsapi.DecoratorEntry {
	return []toolsapi.DecoratorEntry{
		toolsapi.RegisterDecorator("file-read", toolsapi.SimpleDecorator{
			Icon: "📄", Label: "Read", Color: "#82AAFF",
			TitleFromInput: pathFromInput,
		}),
		toolsapi.RegisterDecorator("file-write", toolsapi.SimpleDecorator{
			Icon: "📝", Label: "Write", Color: "#C3E88D",
			TitleFromInput: pathFromInput,
		}),
		toolsapi.RegisterDecorator("file-edit", toolsapi.SimpleDecorator{
			Icon: "✏️", Label: "Edit", Color: "#FFCB6B",
			TitleFromInput: pathFromInput,
		}),
		toolsapi.RegisterDecorator("glob", toolsapi.SimpleDecorator{
			Icon: "📂", Label: "Glob", Color: "#89DDFF",
			TitleFromInput: func(input json.RawMessage) string {
				var in struct {
					Pattern string `json:"pattern"`
				}
				json.Unmarshal(input, &in)
				return in.Pattern
			},
		}),
		toolsapi.RegisterDecorator("grep", toolsapi.SimpleDecorator{
			Icon: "🔍", Label: "Grep", Color: "#F78C6C",
			TitleFromInput: func(input json.RawMessage) string {
				var in struct {
					Pattern string `json:"pattern"`
				}
				json.Unmarshal(input, &in)
				return in.Pattern
			},
		}),
		toolsapi.RegisterDecorator("bash", toolsapi.SimpleDecorator{
			Icon: "🐚", Label: "Bash", Color: "#676E95",
			TitleFromInput: func(input json.RawMessage) string {
				var in struct {
					Command string `json:"command"`
				}
				json.Unmarshal(input, &in)
				cmd := in.Command
				if len(cmd) > 60 {
					cmd = cmd[:57] + "..."
				}
				return cmd
			},
		}),
		toolsapi.RegisterDecorator("ask-user", toolsapi.SimpleDecorator{
			Icon: "❓", Label: "Ask", Color: "#C792EA",
			TitleFromInput: func(input json.RawMessage) string {
				var in struct {
					Question string `json:"question"`
				}
				json.Unmarshal(input, &in)
				return in.Question
			},
			SummarizeResult: func(result json.RawMessage) string {
				var r map[string]any
				if json.Unmarshal(result, &r) == nil {
					if answer, ok := r["answer"].(string); ok {
						return fmt.Sprintf("%q", answer)
					}
				}
				return ""
			},
		}),
		toolsapi.RegisterDecorator("complete", toolsapi.SimpleDecorator{
			Icon: "✅", Label: "Done", Color: "#C3E88D",
			TitleFromInput: func(input json.RawMessage) string {
				var in struct {
					Summary string `json:"summary"`
				}
				json.Unmarshal(input, &in)
				if len(in.Summary) > 80 {
					return in.Summary[:77] + "..."
				}
				return in.Summary
			},
		}),
		toolsapi.RegisterDecorator("delegate", toolsapi.SimpleDecorator{
			Icon: "🤖", Label: "Delegate", Color: "#82AAFF",
			TitleFromInput: func(input json.RawMessage) string {
				var in struct {
					TaskName string `json:"task_name"`
					Mode     string `json:"mode"`
				}
				json.Unmarshal(input, &in)
				if in.Mode != "" {
					return fmt.Sprintf("%s (%s)", in.TaskName, in.Mode)
				}
				return in.TaskName
			},
		}),
		toolsapi.RegisterDecorator("web-search", toolsapi.SimpleDecorator{
			Icon: "🌐", Label: "Search", Color: "#89DDFF",
			TitleFromInput: func(input json.RawMessage) string {
				var in struct {
					Query string `json:"query"`
				}
				json.Unmarshal(input, &in)
				return in.Query
			},
		}),
		toolsapi.RegisterDecorator("web-fetch", toolsapi.SimpleDecorator{
			Icon: "🌍", Label: "Fetch", Color: "#89DDFF",
			TitleFromInput: func(input json.RawMessage) string {
				var in struct {
					URL string `json:"url"`
				}
				json.Unmarshal(input, &in)
				return in.URL
			},
		}),
		toolsapi.RegisterDecorator("rest-api-fetch", toolsapi.SimpleDecorator{
			Icon: "🔌", Label: "API", Color: "#F78C6C",
			TitleFromInput: func(input json.RawMessage) string {
				var in struct {
					Endpoint string `json:"endpoint"`
				}
				json.Unmarshal(input, &in)
				return in.Endpoint
			},
		}),
	}
}

// pathFromInput extracts the file path from tool input, trying common field names.
func pathFromInput(input json.RawMessage) string {
	var in struct {
		WorkspacePath string `json:"workspace_path"`
		AbsolutePath  string `json:"absolute_path"`
		FilePath      string `json:"file_path"`
	}
	json.Unmarshal(input, &in)
	if in.WorkspacePath != "" {
		return in.WorkspacePath
	}
	if in.FilePath != "" {
		return in.FilePath
	}
	return in.AbsolutePath
}
