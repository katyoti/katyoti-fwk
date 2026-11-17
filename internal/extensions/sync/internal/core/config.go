// SPDX-License-Identifier: Apache-2.0

package core

// SyncServiceConfig is the global configuration for the sync service.
// Config file: SyncJobService.yaml (named after the service type it configures)
type SyncServiceConfig struct {
	ServerHost string `yaml:"server_host" default:"127.0.0.1"`
	ServerPort int    `yaml:"server_port" default:"8894"`
	LogLevel   string `yaml:"log_level"   default:"info"`

	// Defaults applied to all jobs unless overridden
	Defaults SyncDefaults `yaml:"defaults"`

	// Bisync state directory (relative to home)
	BisyncWorkdir string `yaml:"bisync_workdir" default:"sync/bisync-state"`
}

// SyncDefaults holds default sync options applied to all jobs.
type SyncDefaults struct {
	Conflict   string   `yaml:"conflict"    default:"newest"`
	Transfers  int      `yaml:"transfers"   default:"4"`
	Checkers   int      `yaml:"checkers"    default:"8"`
	RetryCount int      `yaml:"retry_count" default:"3"`
	Bandwidth  string   `yaml:"bandwidth"`
	Exclude    []string `yaml:"exclude"`
}
