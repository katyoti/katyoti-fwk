// SPDX-License-Identifier: Apache-2.0

// Package configure drives an installed sidecar's OWN config surface (the
// `sidecar config check`/`set` verbs) generically — katyoti never hand-writes a
// sidecar's config. Check → materialize config defaults if missing → re-check.
// env/auth/program items are surfaced with their actionable hints (applying
// those is interactive / a richer follow-on).
package configure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
)

const timeout = 30 * time.Second

// Check runs `<binary> sidecar config check --out json` and parses the report.
//
// home is the workspace the sidecar must resolve its config against. Pass "" to
// let it inherit this process's workspace — correct only when the caller really
// does mean "the universe I am running in" (Doctor). Anything acting ON a named
// universe MUST pass that universe's home; see the note on runIn.
func Check(ctx context.Context, home, binaryPath string) (sidecarapi.ConfigCheckReport, error) {
	return CheckWith(ctx, home, binaryPath, nil)
}

// CheckWith is Check with pre-args inserted before the `sidecar …` verb — e.g.
// a content sidecar checked via the generic host: `ktw-host --folder <dir>
// sidecar config check`. pre carries the persistent flags (`--folder <dir>`).
func CheckWith(ctx context.Context, home, binaryPath string, pre []string) (sidecarapi.ConfigCheckReport, error) {
	return runCheck(ctx, home, binaryPath, pre)
}

// Configure drives the sidecar's own config surface: check → if any config-file
// item is missing, materialize the bundled defaults (`config set`) → re-check.
func Configure(ctx context.Context, home, binaryPath string) (sidecarapi.ConfigCheckReport, error) {
	return ConfigureWith(ctx, home, binaryPath, nil)
}

// ConfigureWith is Configure with pre-args (see CheckWith) — the path a content
// sidecar takes through the generic host.
func ConfigureWith(ctx context.Context, home, binaryPath string, pre []string) (sidecarapi.ConfigCheckReport, error) {
	rep, err := runCheck(ctx, home, binaryPath, pre)
	if err != nil {
		return rep, err
	}
	needsSet := false
	for _, it := range rep.Items {
		if it.Category == "config" && it.Status != "ok" {
			needsSet = true
			break
		}
	}
	if needsSet {
		if err := run(ctx, home, binaryPath, append(append([]string{}, pre...), "sidecar", "config", "set")...); err != nil {
			return rep, fmt.Errorf("config set: %w", err)
		}
		return runCheck(ctx, home, binaryPath, pre)
	}
	return rep, nil
}

// Get reads one config file through the sidecar's own `config get` verb:
// `<binary> [pre…] sidecar config get NAME [--default] --out json`. useDefault
// reads the bundled template instead of the live file — the editor's starting
// point when the file does not exist yet.
func Get(ctx context.Context, home, binaryPath string, pre []string, name string, useDefault bool) (sidecarapi.ConfigGetOutput, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append(append([]string{}, pre...), "sidecar", "config", "get", name)
	if useDefault {
		args = append(args, "--default")
	}
	args = append(args, "--out", "json")
	var out sidecarapi.ConfigGetOutput
	if err := runJSON(cctx, home, binaryPath, args, nil, &out); err != nil {
		return out, fmt.Errorf("config get %s: %w", name, err)
	}
	return out, nil
}

// SetContent writes one config file whole through the sidecar's own `config
// set` verb, content on stdin: `<binary> [pre…] sidecar config set --name NAME
// --file - --out json`. The sidecar validates before writing; an invalid
// content comes back as an error and nothing is written.
func SetContent(ctx context.Context, home, binaryPath string, pre []string, name, content string) (sidecarapi.ConfigSetOutput, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append(append([]string{}, pre...), "sidecar", "config", "set", "--name", name, "--file", "-", "--out", "json")
	var out sidecarapi.ConfigSetOutput
	if err := runJSON(cctx, home, binaryPath, args, strings.NewReader(content), &out); err != nil {
		return out, fmt.Errorf("config set %s: %w", name, err)
	}
	return out, nil
}

// runJSON runs the verb and decodes its JSON stdout into out. A non-zero exit
// surfaces the sidecar's stderr (its error message) rather than "exit status 1".
func runJSON(ctx context.Context, home, binaryPath string, args []string, stdin io.Reader, out any) error {
	cmd := runIn(ctx, home, binaryPath, args)
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	raw, err := cmd.Output()
	if err != nil {
		if msg := firstErrorLine(stderr.String()); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	if len(raw) == 0 {
		return fmt.Errorf("empty response")
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("unparseable response: %w", err)
	}
	return nil
}

// firstErrorLine reduces a Cobra failure's stderr — "Error: <msg>" followed by
// the whole usage block — to <msg>. That is what a GUI toast or a CLI caller
// should see, not the flag table of a binary the user never typed.
func firstErrorLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return strings.TrimPrefix(line, "Error: ")
	}
	return ""
}

func runCheck(ctx context.Context, home, binaryPath string, pre []string) (sidecarapi.ConfigCheckReport, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := append(append([]string{}, pre...), "sidecar", "config", "check", "--out", "json")
	out, err := runIn(cctx, home, binaryPath, args).Output()
	var rep sidecarapi.ConfigCheckReport
	// `config check` may exit non-zero when unhealthy but still print the JSON
	// report to stdout — prefer parsing the output over the exit code.
	if len(out) > 0 {
		if jerr := json.Unmarshal(out, &rep); jerr == nil {
			return rep, nil
		}
	}
	if err != nil {
		return rep, fmt.Errorf("config check %s: %w", binaryPath, err)
	}
	return rep, fmt.Errorf("config check %s: empty/unparseable report", binaryPath)
}

func run(ctx context.Context, home, binaryPath string, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return runIn(cctx, home, binaryPath, args).Run()
}

// runIn builds the child command with KTW_WORKSPACE pinned to home.
//
// Without this the child simply INHERITS the parent's workspace, and a sidecar
// resolves its config dir from the environment. That is wrong wherever katyoti
// is acting on a universe it is not itself running in — and over KTSP it always
// is: these methods are served by the ADMIN universe's hub. Bootstrapping a
// release therefore materialized every member's config templates
// (NotifyService.yaml, TTSVoices.yaml, …) into ADMIN instead of the universe
// being created. Same root cause as the empty-universeHome bug in
// installSolution: "the process's own workspace" standing in for a named
// target.
func runIn(ctx context.Context, home, binaryPath string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	if home != "" {
		cmd.Env = append(os.Environ(), "KTW_WORKSPACE="+home)
	}
	return cmd
}
