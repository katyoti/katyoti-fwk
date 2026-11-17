// SPDX-License-Identifier: Apache-2.0

// Package client provides typed clients for calling sidecar services.
//
// CLIClient is the BOOTSTRAP client: it runs the sidecar binary via exec.Command
// and parses --out json output. No KTSP, no server, no configuration required.
// Used by ktw-agents' config/check commands and by the central installer to
// discover and configure sidecars before KTSP is available.
//
// Once a sidecar is configured and serving, the standard KTSP mechanism takes over.
// The CLIClient is only for the bootstrap/configuration phase.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
)

// CLIClient calls sidecar services by running the binary via exec.Command.
// This is the bootstrap path — works before KTSP is configured.
// Implements api.Sidecar and api.SidecarConfigService.
type CLIClient struct {
	// BinaryPath is the resolved filesystem path to the sidecar binary.
	BinaryPath string

	// Timeout is the maximum duration for a single command execution.
	// Defaults to 30 seconds if zero.
	Timeout time.Duration
}

// NewCLIClient creates a CLIClient for the given binary path.
func NewCLIClient(binaryPath string) *CLIClient {
	return &CLIClient{
		BinaryPath: binaryPath,
		Timeout:    30 * time.Second,
	}
}

// Discover creates a CLIClient by looking up the binary on PATH.
// Returns an error if the binary is not found.
func Discover(binaryName string) (*CLIClient, error) {
	path, err := exec.LookPath(binaryName)
	if err != nil {
		return nil, fmt.Errorf("sidecar %q not found on PATH: %w", binaryName, err)
	}
	return NewCLIClient(path), nil
}

// --- api.Sidecar implementation ---

// Info calls `<sidecar> sidecar info --out json` and returns the InfoReport.
func (c *CLIClient) Info(ctx context.Context) (api.InfoReport, error) {
	var report api.InfoReport
	if err := c.run(ctx, &report, "sidecar", "info"); err != nil {
		return report, fmt.Errorf("sidecar info: %w", err)
	}
	return report, nil
}

// --- api.SidecarConfigService implementation ---
// (Describe was removed 2026-05-10; data moved to ResourceService.
// CLIClient consumers that need that data should call `<sidecar>
// resources list --type config-file/config-program/env` directly or
// go through the ResourceService surface.)

// Get calls `<sidecar> sc config get <name> [--default] --out json`.
// With in.Default=true the binary reads the bundled embedded default
// instead of the on-disk file.
func (c *CLIClient) Get(ctx context.Context, in api.ConfigGetInput) (api.ConfigGetOutput, error) {
	args := []string{"sc", "config", "get", in.Name}
	if in.Default {
		args = append(args, "--default")
	}
	var out api.ConfigGetOutput
	if err := c.run(ctx, &out, args...); err != nil {
		return out, fmt.Errorf("config get: %w", err)
	}
	return out, nil
}

// Validate calls `<sidecar> sc config validate <name> --out json` with content on stdin.
func (c *CLIClient) Validate(ctx context.Context, in api.ConfigValidateInput) (api.ConfigValidateOutput, error) {
	var out api.ConfigValidateOutput
	if err := c.runWithStdin(ctx, &out, marshalJSON(in), "sc", "config", "validate", in.Name); err != nil {
		return out, fmt.Errorf("config validate: %w", err)
	}
	return out, nil
}

// Set calls `<sidecar> sc config set [--reset] --out json`.
// Replaces the former Materialize (renamed 2026-05-23).
func (c *CLIClient) Set(ctx context.Context, in api.ConfigSetInput) (api.ConfigSetOutput, error) {
	args := []string{"sc", "config", "set"}
	if in.Reset {
		args = append(args, "--reset")
	}
	var out api.ConfigSetOutput
	if err := c.run(ctx, &out, args...); err != nil {
		return out, fmt.Errorf("config set: %w", err)
	}
	return out, nil
}

// Check calls `<sidecar> sc config check --out json`.
func (c *CLIClient) Check(ctx context.Context) (api.ConfigCheckReport, error) {
	var out api.ConfigCheckReport
	if err := c.run(ctx, &out, "sc", "config", "check"); err != nil {
		return out, fmt.Errorf("config check: %w", err)
	}
	return out, nil
}

// Setup calls `<sidecar> sc config setup --out json`.
// Note: interactive setup via CLI client is limited — the binary's stdin
// is not connected. Use this only for non-interactive (--reset) mode,
// or run the binary directly for interactive setup. Replaces the former
// Do (renamed 2026-05-23).
func (c *CLIClient) Setup(ctx context.Context, in api.ConfigSetupInput) (api.ConfigSetupOutput, error) {
	args := []string{"sc", "config", "setup"}
	if in.Reset {
		args = append(args, "--reset")
	}
	var out api.ConfigSetupOutput
	if err := c.run(ctx, &out, args...); err != nil {
		return out, fmt.Errorf("config setup: %w", err)
	}
	return out, nil
}

// --- Internal helpers ---

// run executes the sidecar binary with `--out json` and unmarshals the result.
func (c *CLIClient) run(ctx context.Context, result any, args ...string) error {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fullArgs := append(args, "--out", "json")
	cmd := exec.CommandContext(ctx, c.BinaryPath, fullArgs...)

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("exit %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return err
	}

	if result != nil {
		if err := json.Unmarshal(out, result); err != nil {
			return fmt.Errorf("parse JSON: %w", err)
		}
	}
	return nil
}

// runWithStdin is like run but pipes input to the command's stdin.
func (c *CLIClient) runWithStdin(ctx context.Context, result any, stdin []byte, args ...string) error {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fullArgs := append(args, "--out", "json")
	cmd := exec.CommandContext(ctx, c.BinaryPath, fullArgs...)
	cmd.Stdin = bytesReader(stdin)

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("exit %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return err
	}

	if result != nil {
		if err := json.Unmarshal(out, result); err != nil {
			return fmt.Errorf("parse JSON: %w", err)
		}
	}
	return nil
}

func marshalJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

// bytesReader wraps a byte slice as an io.Reader.
type bytesReaderImpl struct {
	data []byte
	pos  int
}

func bytesReader(data []byte) *bytesReaderImpl {
	return &bytesReaderImpl{data: data}
}

func (r *bytesReaderImpl) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
