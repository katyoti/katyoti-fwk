// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// ClaudeCode starts, follows and answers the Claude Code workers of this process.
type ClaudeCode interface {
	// Start spawns a worker, completes its handshake (the subscription guard
	// included) and sends its first prompt.
	Start(ctx context.Context, in StartInput) (Worker, error)
	// Send writes the next user message (queued by Claude Code during a turn).
	Send(ctx context.Context, in SendInput) (Worker, error)
	// Answer answers the permission the worker waits on.
	Answer(ctx context.Context, in AnswerInput) (Worker, error)
	// Interrupt ends the current turn.
	Interrupt(ctx context.Context, in WorkerInput) (Worker, error)
	// Stop ends the worker cleanly.
	Stop(ctx context.Context, in WorkerInput) (Worker, error)
	// Get reads one worker.
	Get(ctx context.Context, in WorkerInput) (Worker, error)
	// List reads the workers this process holds.
	List(ctx context.Context, in ListInput) (ListOutput, error)
}

// ServiceDef describes the Claude Code service.
var ServiceDef = meta.ServiceDef{
	Name:  "claude",
	Short: "Claude Code workers on the person's own subscription",
	Long: `Starts Claude Code workers - the claude CLI driven over stdio as the Agent SDKs do -
on the person's own Claude subscription (an API key is refused), follows their state and
answers their permissions. The protocol: modules/claude/docs/claude-code-protocol.md.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Start":     meta.Method("Start a Claude Code worker on a prompt, in a working directory"),
		"Send":      meta.Method("Send a worker its next user message"),
		"Answer":    meta.Method("Answer the permission a worker waits on: allow or deny, once or for the session or the project"),
		"Interrupt": meta.Method("End a worker's current turn"),
		"Stop":      meta.Method("End a worker cleanly"),
		"Get":       meta.Method("Read one worker: its state, its pending permission, its cost"),
		"List":      meta.Method("List the workers this process holds"),
	},
}

// Follower is what the in-process service adds: a worker's events so far and
// from then on (the foreground run prints them).
type Follower interface {
	Follow(id string) ([]Event, <-chan Event, func(), error)
}
