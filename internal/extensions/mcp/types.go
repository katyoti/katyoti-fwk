// SPDX-License-Identifier: Apache-2.0

package mcp

// MCPServerConfig describes an MCP server to connect to.
type MCPServerConfig struct {
	Name      string `yaml:"name" json:"name"`
	Command   string `yaml:"command" json:"command"`
	URL       string `yaml:"url" json:"url"`
	Transport string `yaml:"transport" json:"transport"` // "stdio" or "http"
}
