// SPDX-License-Identifier: Apache-2.0

package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/file_edit"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/file_read"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/file_write"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/glob"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/grep"
)

// StrategyDef is the typed strategy definition for the agent.plugin domain.
var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"workspace", "Sandboxed file I/O",
	func() (agentapi.PluginFactory, error) {
		return workspaceFactory("workspace", ""), nil
	},
)

func workspaceFactory(name, wsRoot string) agentapi.PluginFactory {
	return &agentapi.FuncFactory{
		FactoryName: name,
		CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
			root := wsRoot
			if root == "" {
				root = application.ProjectRoot() // the project the universe serves
			}
			ws, err := newWorkspace(root)
			if err != nil {
				return nil, err
			}
			return []agentapi.AgentPlugin{newWorkspacePlugin(ws)}, nil
		},
	}
}

// workspace manages sandboxed file I/O for the LLM.
// It implements PathResolver for use by tool packages.
type workspace struct {
	root string
}

func newWorkspace(root string) (*workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	return &workspace{root: abs}, nil
}

// WorkspaceRoot returns the canonicalized absolute path of the workspace
// root. Used by tools (e.g. file_read) that need to classify
// absolute_path inputs against the workspace boundary for permission
// checks.
func (w *workspace) WorkspaceRoot() string {
	return w.root
}

// Resolve validates and resolves a relative path within the workspace.
func (w *workspace) Resolve(relPath string) (string, error) {
	if relPath == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "/") {
		return "", fmt.Errorf("absolute paths not allowed: %s", relPath)
	}
	cleaned := filepath.Clean(relPath)
	if strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("path escapes workspace: %s", relPath)
	}
	abs := filepath.Join(w.root, cleaned)
	if !strings.HasPrefix(abs, w.root) {
		return "", fmt.Errorf("path escapes workspace: %s", relPath)
	}
	return abs, nil
}

// newWorkspacePlugin is the composite plugin of the file tools and glob/grep.
// It is a toolsapi tool-group plugin so the agent runtime sees each Tool: the
// permission gate looks a tool up by name to apply the profile's rules and the
// tool's own CheckPermissions — a plugin that hid its tools let every call
// through. The system message is one terse line: workspace-relative paths are
// the PREFERRED method (no friction); absolute paths are allowed but require
// user approval. The per-tool mechanics live in each file tool's description.
func newWorkspacePlugin(ws *workspace) agentapi.AgentPlugin {
	return toolsapi.NewToolGroupPlugin("workspace", systemMessage, []toolsapi.Tool{
		file_read.NewFileReadTool(ws),
		file_write.NewFileWriteTool(ws),
		file_edit.NewFileEditTool(ws),
		glob.NewGlobTool(ws),
		grep.NewGrepTool(ws),
	})
}

const systemMessage = "## Workspace\n\n" +
	"Prefer **workspace-relative** paths (`workspace_path`) for files in the project — " +
	"that's the fast, frictionless default. Absolute paths are allowed but require user approval.\n\n"
