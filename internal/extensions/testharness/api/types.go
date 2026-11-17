// SPDX-License-Identifier: Apache-2.0

package api

import "time"

// RunRequest describes a process to spawn.
type RunRequest struct {
	Cmd  string   `json:"cmd"`            // executable path
	Args []string `json:"args,omitempty"` // command-line arguments
	Dir  string   `json:"dir,omitempty"`  // working directory (default: cwd)
	Env  []string `json:"env,omitempty"`  // extra env vars ("KEY=VALUE")
}

// RunResponse is returned after spawning a process.
type RunResponse struct {
	SessionID string `json:"sessionId"`
	PID       int    `json:"pid"`
}

// OutputLine is a single captured line from stdout or stderr.
type OutputLine struct {
	Seq    int64     `json:"seq"`
	Ts     time.Time `json:"ts"`
	Stream string    `json:"stream"` // "stdout" | "stderr"
	Text   string    `json:"text"`
}

// InputRequest sends text to the process stdin.
type InputRequest struct {
	Text string `json:"text"`
}

// SessionStatus reports the current state of the spawned process.
type SessionStatus struct {
	SessionID string `json:"sessionId"`
	Running   bool   `json:"running"`
	PID       int    `json:"pid"`
	ExitCode  *int   `json:"exitCode"`
	OutputLen int    `json:"outputLines"`
	StartedAt string `json:"startedAt,omitempty"`
	Cmd       string `json:"cmd"`
}
