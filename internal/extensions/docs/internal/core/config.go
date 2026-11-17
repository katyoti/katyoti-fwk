// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
)

// DocServiceConfig holds configuration for the ktw-docs services.
// Config file: DocService.yaml (ConfigName strips "Config" suffix).
type DocServiceConfig struct {
	ServerHost string `yaml:"server_host" default:"127.0.0.1"`
	ServerPort int    `yaml:"server_port" default:"8890"`
	KrokiURL   string `yaml:"kroki_url" default:"https://kroki.io"`
	LogLevel   string `yaml:"log_level" default:"info"`
}

// Addr returns the host:port address string.
func (s DocServiceConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.ServerHost, s.ServerPort)
}
