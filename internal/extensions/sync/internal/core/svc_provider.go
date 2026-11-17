// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	syncapi "github.com/katyoti/katyoti-fwk/internal/extensions/sync/api"
)

// providerService implements syncapi.SyncProviderService.
type providerService struct{}

func (s *providerService) List(_ context.Context, in syncapi.ListProvidersInput) (syncapi.ListProvidersOutput, error) {
	return ListProviders(in.Query), nil
}

// Login runs rclone authorize for the given backend type.
// This opens a browser for OAuth login and returns the token.
func (s *providerService) Login(_ context.Context, in syncapi.LoginInput) (syncapi.LoginOutput, error) {
	if in.Backend == "" {
		return syncapi.LoginOutput{}, fmt.Errorf("backend is required (e.g. drive, onedrive, s3)")
	}

	// Verify the backend exists
	ri := findProvider(in.Backend)
	if ri == nil {
		return syncapi.LoginOutput{}, fmt.Errorf("unknown backend %q — run 'provider list' to see available backends", in.Backend)
	}

	// Run rclone authorize <backend>
	// This opens a browser, user logs in, rclone prints the token.
	// Capture both stdout and stderr — rclone mixes output across both.
	cmd := exec.Command("rclone", "authorize", in.Backend)
	cmd.Stdin = os.Stdin

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	fmt.Fprintf(os.Stderr, "Authorizing %s — this will open your browser...\n", in.Backend)

	if err := cmd.Run(); err != nil {
		return syncapi.LoginOutput{
			Status:  "error",
			Message: fmt.Sprintf("rclone authorize failed: %v\n%s", err, combined.String()),
		}, nil
	}

	// Extract the token from rclone output.
	output := combined.String()
	token := extractToken(output)

	if token == "" {
		return syncapi.LoginOutput{
			Status:  "ok",
			Message: "Authorization completed but could not extract token automatically. Check rclone output.",
		}, nil
	}

	return syncapi.LoginOutput{
		Status:  "ok",
		Token:   token,
		Message: fmt.Sprintf("Login successful. Paste this token into your source config under workspace.token:\n  token: '%s'", token),
	}, nil
}

// extractToken finds the JSON token in rclone authorize output.
// rclone prints it between marker lines.
func extractToken(output string) string {
	const startMarker = "Paste the following into your remote machine --->"
	const endMarker = "<--- End paste"

	startIdx := strings.Index(output, startMarker)
	if startIdx < 0 {
		// Try to find raw JSON token
		if idx := strings.Index(output, `{"access_token"`); idx >= 0 {
			end := strings.Index(output[idx:], "}")
			if end > 0 {
				return strings.TrimSpace(output[idx : idx+end+1])
			}
		}
		return ""
	}

	startIdx += len(startMarker)
	endIdx := strings.Index(output[startIdx:], endMarker)
	if endIdx < 0 {
		return ""
	}

	return strings.TrimSpace(output[startIdx : startIdx+endIdx])
}
