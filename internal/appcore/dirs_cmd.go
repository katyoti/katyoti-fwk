// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	"github.com/spf13/cobra"
)

// DirsOutput is the machine-readable shape behind `<binary> sc dirs`.
// Rendered as labeled rows in rich mode (the historical look) and as
// JSON in `--out json` mode. The struct is the contract — every key
// here is a stable JSON field for tooling consumers.
type DirsOutput struct {
	Home         string         `json:"home"`
	GlobalHome   string         `json:"global_home"`
	Data         string         `json:"data"`
	Cache        string         `json:"cache"`
	Workspace    string         `json:"workspace"`
	UniverseName string         `json:"universe_name"`
	UniverseOK   bool           `json:"universe_ok"`
	UniverseErr  string         `json:"universe_err,omitempty"`
	Config       string         `json:"config"`
	Skills       string         `json:"skills"`
	Tasks        string         `json:"tasks"`
	Sync         string         `json:"sync"`
	Sessions     string         `json:"sessions"`
	Servers      string         `json:"servers"`
	Logs         string         `json:"logs"`
	EnvFiles     []DirsEnvEntry `json:"env_files,omitempty"`
	// Installation is the installation file this process belongs to ("" for
	// none: a development build), and Refused the inherited variables it did
	// not take because they came from outside it.
	Installation string              `json:"installation,omitempty"`
	Refused      []cenv.RefusedInput `json:"refused_inputs,omitempty"`
}

// DirsEnvEntry describes one `.env` file the binary searched for or
// loaded at startup. Loaded=true means the file was actually read into
// the process environment; Withheld are the keys it holds that it did not
// set, another scope's (not declared in cenv.GlobalKeysVar).
type DirsEnvEntry struct {
	Path     string   `json:"path"`
	Loaded   bool     `json:"loaded"`
	Withheld []string `json:"withheld,omitempty"`
}

const dirsTemplate = `{{ header "Roots" }}{{ row "home" .Home }}{{ row "global home" .GlobalHome }}{{ row "data" .Data }}{{ row "cache" .Cache }}{{ row "workspace" .Workspace }}
{{ header "Universe" }}{{ row "name" .UniverseName }}{{ if .UniverseOK }}{{ row "status" "ok" }}{{ else }}{{ row "status" .UniverseErr }}{{ end }}
{{ header "Config (under home)" }}{{ row "config" .Config }}{{ row "skills" .Skills }}{{ row "tasks" .Tasks }}{{ row "sync" .Sync }}
{{ header "Data (under data)" }}{{ row "sessions" .Sessions }}{{ row "servers" .Servers }}{{ row "logs" .Logs }}
{{ header "Installation" }}{{ if .Installation }}{{ row "file" .Installation }}{{ else }}{{ dim "none: a development build or a test" }}{{ end }}{{ range .Refused }}{{ warn (printf "refused %s=%s: %s" .Name .Value .Why) }}{{ end }}
{{- if .EnvFiles }}{{ header "Environment files" }}{{ range .EnvFiles }}{{ if .Loaded }}{{ ok (printf "loaded: %s" .Path) }}{{ range .Withheld }}{{ warn (printf "withheld %s: another scope's key, not declared in KTW_GLOBAL_KEYS" .) }}{{ end }}{{ else }}{{ dim (printf "searched: %s" .Path) }}{{ end }}{{ end }}{{ end }}`

// NewDirsCommand builds the `dirs` cobra command that prints all
// resolved directories. Lives under `sc` (relocated by SidecarExt's
// promote step). Supports `--out rich|raw|json` for layout uniformity
// with the rest of the sc surface.
func NewDirsCommand() *cobra.Command {
	var openFlag bool

	cmd := &cobra.Command{
		Use:   "dirs",
		Short: "Show all resolved directories",
		Long:  "Display the resolved paths for config, data, cache directories and loaded .env files.\nUse --open to open the 3 root folders in the file manager.",
	}
	mode := console.AddOutputFlag(cmd)
	cmd.Flags().BoolVar(&openFlag, "open", false, "Open the 3 root folders in the file manager")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		env := application.Env()
		out := DirsOutput{
			Home:         env.HomeDir(),
			GlobalHome:   env.GlobalHomeDir(),
			Data:         env.DataDir(),
			Cache:        env.CacheDir(),
			Workspace:    env.WorkspaceDir(),
			UniverseName: env.UniverseName(),
			Config:       env.ConfigDir(),
			Skills:       env.GetHomeSubDir("skills"),
			Tasks:        env.GetHomeSubDir("tasks"),
			Sync:         env.GetHomeSubDir("sync"),
			Sessions:     env.SessionsDir(),
			Servers:      env.ServersDir(),
			Logs:         env.LogDir(),
		}
		if uErr := env.UniverseError(); uErr != nil {
			out.UniverseErr = uErr.Error()
		} else {
			out.UniverseOK = true
		}
		if inst := cenv.CurrentInstallation(); inst != nil {
			out.Installation = inst.Path()
		}
		out.Refused = cenv.RefusedInputs()
		for _, s := range cenv.EnvFileStatuses() {
			out.EnvFiles = append(out.EnvFiles, DirsEnvEntry{Path: s.Path, Loaded: s.Loaded, Withheld: s.Withheld})
		}

		if err := console.RenderConsole(*mode, "dirs", out, dirsTemplate); err != nil {
			return err
		}

		if openFlag {
			w := cmd.OutOrStdout()
			fmt.Fprintln(w)
			for _, dir := range []string{out.Home, out.Data, out.Cache} {
				_ = os.MkdirAll(dir, 0o755)
				if err := openInFileManager(dir); err != nil {
					fmt.Fprintf(w, "  failed to open %s: %v\n", dir, err)
				}
			}
		}
		return nil
	}

	return cmd
}

// openInFileManager opens a directory in the platform's file manager.
func openInFileManager(dir string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	default: // linux, freebsd, etc.
		return exec.Command("xdg-open", dir).Start()
	}
}
