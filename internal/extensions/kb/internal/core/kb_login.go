// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

func (s *kbService) Login(ctx context.Context, in kbapi.LoginInput) (kbapi.LoginOutput, error) {
	if in.Path == "" {
		return kbapi.LoginOutput{}, fmt.Errorf("path is required")
	}
	if in.Source == "" {
		return kbapi.LoginOutput{}, fmt.Errorf("source is required")
	}

	absPath, err := application.ProjectPath(in.Path)
	if err != nil {
		return kbapi.LoginOutput{}, fmt.Errorf("resolve path: %w", err)
	}

	srcCfg, err := LoadSource(absPath, in.Source)
	if err != nil {
		return kbapi.LoginOutput{}, err
	}

	wsType, _ := srcCfg.Workspace["type"].(string)
	if wsType == "" {
		return kbapi.LoginOutput{}, fmt.Errorf("source %s has no workspace.type", in.Source)
	}

	if wsType == "local" {
		return kbapi.LoginOutput{
			Source:  in.Source,
			Message: "Local source — no login needed",
		}, nil
	}

	// Call ktw-sync provider login for this workspace type.
	// This runs rclone's OAuth flow interactively (opens browser).
	binPath := resolveSidecarBinary("ktw-sync")

	cmd := exec.CommandContext(ctx, binPath, "provider", "login", "--backend", wsType)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Login for source '%s' (provider: %s)...\n\n", in.Source, wsType)

	if err := cmd.Run(); err != nil {
		return kbapi.LoginOutput{}, fmt.Errorf("login failed: %w", err)
	}

	return kbapi.LoginOutput{
		Source:  in.Source,
		Message: fmt.Sprintf("Login complete. Paste the token into sources/%s.yaml under workspace.token.", in.Source),
	}, nil
}
