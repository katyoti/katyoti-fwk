// SPDX-License-Identifier: Apache-2.0

package orchestration

import "fmt"

// EligibilityResult indicates whether remote execution is available.
type EligibilityResult struct {
	Eligible       bool     `json:"eligible"`
	ErrorMessage   string   `json:"errorMessage,omitempty"`
	MissingPrereqs []string `json:"missingPrereqs,omitempty"`
}

// RemoteExecutor manages remote agent execution.
// Phase 1: stub implementation — always returns ineligible.
// Phase 2 (ms05): full implementation with cloud session teleport.
type RemoteExecutor struct{}

// NewRemoteExecutor creates a remote executor (stub).
func NewRemoteExecutor() *RemoteExecutor {
	return &RemoteExecutor{}
}

// CheckEligibility checks if remote execution is available.
// Stub: always returns ineligible.
func (r *RemoteExecutor) CheckEligibility() EligibilityResult {
	return EligibilityResult{
		Eligible:       false,
		ErrorMessage:   "Remote execution not configured. This feature will be available in a future release.",
		MissingPrereqs: []string{"remote_backend", "authentication"},
	}
}

// SpawnRemote starts a remote agent session.
// Stub: returns error.
func (r *RemoteExecutor) SpawnRemote(taskName string, input map[string]any) (string, error) {
	return "", fmt.Errorf("remote execution not available: %s", r.CheckEligibility().ErrorMessage)
}
