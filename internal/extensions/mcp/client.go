// SPDX-License-Identifier: Apache-2.0

// Package mcp provides an MCP client bridge that connects to configured MCP
// servers and exposes their tools through the ktw-agents tool dispatcher.
package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Manager manages connections to one or more MCP servers.
type Manager struct {
	mu       sync.Mutex
	sessions []*ServerConn
}

// ServerConn holds a live connection to an MCP server.
type ServerConn struct {
	Name    string
	Session *mcpsdk.ClientSession
	Tools   []*mcpsdk.Tool
}

// NewManager creates a new MCP Manager.
func NewManager() *Manager {
	return &Manager{}
}

// ConnectAll connects to all configured MCP servers.
// Errors on individual servers are logged but do not prevent other connections.
func (m *Manager) ConnectAll(ctx context.Context, servers []MCPServerConfig) error {
	for _, srv := range servers {
		conn, err := m.connect(ctx, srv)
		if err != nil {
			slog.Warn("mcp: failed to connect", "server", srv.Name, "error", err)
			continue
		}
		m.mu.Lock()
		m.sessions = append(m.sessions, conn)
		m.mu.Unlock()
		slog.Debug("mcp: connected", "server", srv.Name, "tools", len(conn.Tools))
	}
	return nil
}

func (m *Manager) connect(ctx context.Context, srv MCPServerConfig) (*ServerConn, error) {
	transport, err := m.buildTransport(srv)
	if err != nil {
		return nil, err
	}

	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: "ktw-agents", Version: "1.0.0"},
		nil,
	)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", srv.Name, err)
	}

	// Discover tools
	var tools []*mcpsdk.Tool
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			slog.Warn("mcp: error listing tools", "server", srv.Name, "error", err)
			break
		}
		tools = append(tools, tool)
	}

	return &ServerConn{
		Name:    srv.Name,
		Session: session,
		Tools:   tools,
	}, nil
}

func (m *Manager) buildTransport(srv MCPServerConfig) (mcpsdk.Transport, error) {
	switch srv.Transport {
	case "stdio", "":
		parts := strings.Fields(srv.Command)
		if len(parts) == 0 {
			return nil, fmt.Errorf("empty command for stdio server %s", srv.Name)
		}
		cmd := exec.Command(parts[0], parts[1:]...)
		return &mcpsdk.CommandTransport{Command: cmd}, nil
	case "http":
		if srv.URL == "" {
			return nil, fmt.Errorf("no URL for http server %s", srv.Name)
		}
		return &mcpsdk.StreamableClientTransport{Endpoint: srv.URL}, nil
	default:
		return nil, fmt.Errorf("unknown transport %q for server %s", srv.Transport, srv.Name)
	}
}

// Sessions returns all active server connections.
func (m *Manager) Sessions() []*ServerConn {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*ServerConn, len(m.sessions))
	copy(out, m.sessions)
	return out
}

// Close closes all MCP server connections.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sc := range m.sessions {
		if err := sc.Session.Close(); err != nil {
			slog.Debug("mcp: close error", "server", sc.Name, "error", err)
		}
	}
	m.sessions = nil
}
