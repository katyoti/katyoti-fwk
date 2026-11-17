// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CheckReport is the standard JSON report format for the sidecar contract.
// Every ktw-* sidecar's `check` command MUST output this structure.
type CheckReport struct {
	Deps        []DepEntry        `json:"deps"`
	EnvKeys     []EnvKeyEntry     `json:"env_keys"`
	ConfigFiles []ConfigFileEntry `json:"config_files"`
	Commands    []CommandEntry    `json:"commands,omitempty"`
}

// DepEntry is now defined in core/publish/toolcheck.go and re-exported via manifest.go.

// EnvKeyEntry describes a required or optional environment variable.
type EnvKeyEntry struct {
	Key         string `json:"key"`
	Required    bool   `json:"required"`
	Set         bool   `json:"set"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

// ConfigFileEntry describes a config file managed by the sidecar.
type ConfigFileEntry struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	ManagedBy string `json:"managed_by"`
}

// CommandEntry describes a CLI command available on the sidecar.
type CommandEntry struct {
	Name  string         `json:"name"`
	Short string         `json:"short"`
	Flags []FlagEntry    `json:"flags,omitempty"`
	Sub   []CommandEntry `json:"sub,omitempty"`
}

// FlagEntry describes a CLI flag on a command.
type FlagEntry struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
	Usage     string `json:"usage"`
}

// CommandSchema builds a CommandEntry tree from a Cobra command.
// It skips internal commands (help, completion, check).
func CommandSchema(cmd *cobra.Command) []CommandEntry {
	var entries []CommandEntry
	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" || sub.Name() == "check" {
			continue
		}
		entry := CommandEntry{
			Name:  sub.Name(),
			Short: sub.Short,
			Flags: flagEntries(sub),
		}
		if sub.HasSubCommands() {
			entry.Sub = CommandSchema(sub)
		}
		entries = append(entries, entry)
	}
	return entries
}

func flagEntries(cmd *cobra.Command) []FlagEntry {
	var flags []FlagEntry
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		flags = append(flags, FlagEntry{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Type:      f.Value.Type(),
			Default:   f.DefValue,
			Usage:     f.Usage,
		})
	})
	return flags
}
