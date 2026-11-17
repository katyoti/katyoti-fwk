// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerMCPTools discovers tools from all connected MCP servers and registers
// them directly on the mcpPlugin. Tool names are prefixed with the server name
// to avoid collisions (e.g., "myserver__tool_name").
func registerMCPTools(p *mcpPlugin, mgr *Manager) {
	for _, conn := range mgr.Sessions() {
		for _, tool := range conn.Tools {
			toolName := conn.Name + "__" + tool.Name
			toolDef := convertToolDef(toolName, tool)
			handler := mcpToolHandler(conn.Session, tool.Name)
			p.defs = append(p.defs, toolDef)
			p.handlers[toolName] = handler
			slog.Debug("mcp: registered tool", "name", toolName, "server", conn.Name)
		}
	}
}

// convertToolDef converts an MCP Tool into our internal agentapi.ToolDef.
func convertToolDef(name string, tool *mcpsdk.Tool) agentapi.ToolDef {
	var schemaBytes json.RawMessage
	if tool.InputSchema != nil {
		b, err := json.Marshal(tool.InputSchema)
		if err != nil {
			slog.Warn("mcp: failed to marshal tool schema", "tool", name, "error", err)
			schemaBytes = json.RawMessage(`{"type":"object"}`)
		} else {
			schemaBytes = b
		}
	} else {
		schemaBytes = json.RawMessage(`{"type":"object"}`)
	}

	return agentapi.ToolDef{
		Name:        name,
		Description: tool.Description,
		InputSchema: schemaBytes,
	}
}

// mcpToolHandler creates a handler function that proxies calls to an MCP server.
func mcpToolHandler(session *mcpsdk.ClientSession, toolName string) func(context.Context, json.RawMessage) (string, error) {
	return func(ctx context.Context, input json.RawMessage) (string, error) {
		var args map[string]any
		if len(input) > 0 {
			if err := json.Unmarshal(input, &args); err != nil {
				return "", fmt.Errorf("parse MCP tool input: %w", err)
			}
		}

		result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
			Name:      toolName,
			Arguments: args,
		})
		if err != nil {
			return "", fmt.Errorf("MCP tool %q failed: %w", toolName, err)
		}

		return extractContent(result), nil
	}
}

// extractContent converts MCP CallToolResult content into a string.
func extractContent(result *mcpsdk.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return "{}"
	}

	var parts []string
	for _, c := range result.Content {
		switch content := c.(type) {
		case *mcpsdk.TextContent:
			parts = append(parts, content.Text)
		default:
			b, err := json.Marshal(content)
			if err == nil {
				parts = append(parts, string(b))
			}
		}
	}

	if len(parts) == 1 {
		return parts[0]
	}

	result2 := ""
	for i, p := range parts {
		if i > 0 {
			result2 += "\n"
		}
		result2 += p
	}
	return result2
}
