// SPDX-License-Identifier: Apache-2.0

// Package commands provides grouped Cobra CLI commands for the sidecar contract services.
// These are hand-written (not auto-generated) to ensure grouped presentation
// even on flat-CLI sidecars — following the same pattern as the skills CLI.
package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	"github.com/spf13/cobra"
)

// NewConfigCommand creates the `config` command group with exactly 5
// verbs per the command-surface decision: check, get, set, validate
// setup. Lives under the `sc` cobra group from SidecarGroupsExt.
//
// History (2026-05-23, ms-sidecar-norm):
//   - Dropped: `do` (replaced by `setup`), `materialize` (replaced by
//     `set`), `get-default` (folded into `get --default`), `describe`
//     (data moved to ResourceService.List per the earlier
//     bus-split-config-as-resources).
//   - Renamed: `materialize` → `set`, `do` → `setup`.
func NewConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Configure the sidecar (check / get / set / validate / setup)",
		Long: `Sidecar configuration lifecycle:
  check    — canonical health-check (deps, env, config files, hints)
  get      — read a config file (--default for the embedded default)
  set      — write config (RC1: materializes bundled defaults; --reset for clean rebuild)
  validate — pre-flight validate proposed config content
  setup    — interactive setup (prompts + writes)`,
	}

	cmd.AddCommand(configCheckCmd())
	cmd.AddCommand(configGetCmd())
	cmd.AddCommand(configSetCmd())
	cmd.AddCommand(configValidateCmd())
	cmd.AddCommand(configSetupCmd())

	return cmd
}

// Templates for rich CLI output.
const checkTemplate = `{{- if .Deps}}{{ header "Dependencies" }}{{ table .Deps "Name" "~FoundVersion" "Status:OK" "~InstallSkill" }}{{end}}
{{- if .EnvKeys}}{{ header "Environment Keys" }}{{ table .EnvKeys "EnvName" "Required" "Status:Set" }}{{end}}
{{- if .ConfigFiles}}{{ header "Config Files" }}{{ table .ConfigFiles "Name" "~Path" "Status:Exists" }}{{end}}`

const setTemplate = `{{- header "Set" }}{{ row "Home" .HomeDir }}
{{- if .Dirs}}{{ header "Directories" }}{{ table .Dirs "Path" "Created" }}{{end}}
{{- if .Files}}{{ header "Config Files" }}{{ table .Files "Name" "Status" }}{{end}}`

const setupTemplate = `{{- header "Setup" }}{{ row "Home" .HomeDir }}
{{- if .Dirs}}{{ header "Directories" }}{{ table .Dirs "Path" "Created" }}{{end}}
{{- if .Files}}{{ header "Config Files" }}{{ table .Files "Name" "Status" }}{{end}}
{{- if .Tools}}{{ header "External Tools" }}{{ table .Tools "Name" "OK" "~FoundVersion" }}{{end}}
{{- if .APIKeys}}{{ header "API Keys" }}{{ table .APIKeys "Label" "Status" }}{{end}}`

const getTemplate = `{{- if .Path}}{{ row "Path" .Path }}{{end}}{{ row "File" .Name }}{{ .Content }}`

const validateTemplate = `{{ row "Valid" (printf "%v" .Valid) }}{{- range .Errors}}{{ error . }}{{end}}`

func configCheckCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Health-check (deps, env, config files, hints)",
		Long: `'sc config check' is THE canonical health-check entry. It subsumes
the old top-level 'check' and 'program check' verbs. Output carries
an actionable Hint for every failure so an operator (or LLM) can
follow up directly.

LLM-driven callers: invoke via 'llm tools call config-check {}' to
get the machine-readable CheckReport (Items + Healthy + Summary).`,
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.SidecarConfigService](cmd.Context())
		if err != nil {
			return err
		}
		report, err := svc.Check(cmd.Context())
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "config-check", report, checkTemplate)
	}
	return cmd
}

func configGetCmd() *cobra.Command {
	var useDefault bool
	cmd := &cobra.Command{
		Use:   "get NAME",
		Short: "Read a config file (--default for the embedded default)",
		Long: `Reads a config file by name. With no flag, returns the live
on-disk file. With --default, returns the bundled embedded default
from the sidecar's embed.FS — useful for "show defaults", "reset to
factory", "diff live vs default".`,
		Args: cobra.ExactArgs(1),
	}
	cmd.Flags().BoolVar(&useDefault, "default", false, "Read the bundled embedded default instead of the on-disk file")
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.SidecarConfigService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.Get(cmd.Context(), api.ConfigGetInput{Name: args[0], Default: useDefault})
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "config-get", result, getTemplate)
	}
	return cmd
}

func configSetCmd() *cobra.Command {
	var resetFlag bool
	var key, value, name, file string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Write config to disk (idempotent, non-interactive)",
		Long: `'set' writes config files to disk without prompting. Replaces
the old 'materialize' verb (renamed 2026-05-23).

Two paths:
  set                          materialize every missing bundled default
  set --name F --file P        write P's content as the whole file F
                               ('-' reads stdin); validated first, and
                               nothing is written when it is invalid

The --key/--value flags are placeholders for the structured per-key
writes coming in a follow-up milestone.`,
	}
	cmd.Flags().BoolVar(&resetFlag, "reset", false, "Delete and recreate config files before writing")
	cmd.Flags().StringVar(&name, "name", "", "Config file to write as a whole (with --file)")
	cmd.Flags().StringVar(&file, "file", "", "Path holding the new content for --name ('-' = stdin)")
	cmd.Flags().StringVar(&key, "key", "", "(RFU) Specific key path to write")
	cmd.Flags().StringVar(&value, "value", "", "(RFU) Value to write at the given key")
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if (name == "") != (file == "") {
			return fmt.Errorf("--name and --file go together")
		}
		content := ""
		if file != "" {
			var data []byte
			var err error
			if file == "-" {
				data, err = io.ReadAll(cmd.InOrStdin())
			} else {
				data, err = os.ReadFile(file)
			}
			if err != nil {
				return fmt.Errorf("read --file %s: %w", file, err)
			}
			content = string(data)
		}
		svc, err := application.UseService[api.SidecarConfigService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.Set(cmd.Context(), api.ConfigSetInput{Reset: resetFlag, Name: name, Content: content, Key: key, Value: value})
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "config-set", result, setTemplate)
	}
	return cmd
}

func configValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate NAME",
		Short: "Validate a proposed config (no write)",
		Args:  cobra.ExactArgs(1),
	}
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.SidecarConfigService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.Validate(cmd.Context(), api.ConfigValidateInput{Name: args[0]})
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "config-validate", result, validateTemplate)
	}
	return cmd
}

func configSetupCmd() *cobra.Command {
	var resetFlag bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactive setup (prompts + writes)",
		Long: `'setup' runs the interactive sidecar setup flow: writes bundled
defaults, then prompts for missing API keys (when run interactively).
Replaces the old 'do' verb (renamed 2026-05-23).

For non-interactive bulk material, prefer 'sc config set --reset'.`,
	}
	cmd.Flags().BoolVar(&resetFlag, "reset", false, "Delete and recreate config files")
	mode := console.AddOutputFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		svc, err := application.UseService[api.SidecarConfigService](cmd.Context())
		if err != nil {
			return err
		}
		result, err := svc.Setup(cmd.Context(), api.ConfigSetupInput{Reset: resetFlag})
		if err != nil {
			return err
		}
		return console.RenderConsole(*mode, "config-setup", result, setupTemplate)
	}
	return cmd
}
