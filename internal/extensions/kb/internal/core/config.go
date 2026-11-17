// SPDX-License-Identifier: Apache-2.0

package core

// KBServiceConfig holds the ktw-kb sidecar configuration.
type KBServiceConfig struct {
	ServerHost string `yaml:"server_host" default:"127.0.0.1"`
	ServerPort int    `yaml:"server_port" default:"8896"`
	LogLevel   string `yaml:"log_level" default:"info"`
}
