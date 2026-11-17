// SPDX-License-Identifier: Apache-2.0

package startup

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// Extension contributes the `katyoti config` command to the root command.
func Extension() application.Extension {
	return appcore.CommandExt(Command())
}

// Command builds `config` (get / set / path) — the universe-independent startup
// config (launch mode, default universe, …). Distinct from `configure <item>`,
// which drives an installed sidecar's own config check.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read or set katyoti's global startup config (launch mode, default universe)",
	}
	cmd.AddCommand(newConfigGetCommand(), newConfigSetCommand(), newConfigPathCommand())
	return cmd
}

func newConfigGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Print the resolved startup config",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := Load()
			if err != nil {
				return err
			}
			out, err := yaml.Marshal(&cfg)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
}

func newConfigSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set one startup key (launch_mode | default_universe | autostart_hub | projects_folder) or the installation's lock (locked on|off)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := Set(args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "set %s=%s → %s\n", args[0], args[1], Path())
			_ = cfg
			return nil
		},
	}
}

func newConfigPathCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), Path())
			return nil
		},
	}
}
