// SPDX-License-Identifier: Apache-2.0

package main

// RunningSession is the wire shape the frontend renders for one live
// session. Mirrors terminalapi.SessionInfo (which mirrors
// terminal.Session) — kept local so the Wails-bound surface is
// self-contained and decoupled from the protocol package.
type RunningSession struct {
	ID              string `json:"id"`
	ParentSessionID string `json:"parentSessionId,omitempty"`
	Name            string `json:"name"`
	Status          string `json:"status"` // running | done | error | stopped
	TaskID          string `json:"taskId"`
	ParentTaskID    string `json:"parentTaskId,omitempty"`
	MessageCount    int    `json:"messageCount"`
	StartedAt       string `json:"startedAt"`
}

// ListRunningSessionsResult wraps the live session snapshot + an optional
// error so the frontend can render a banner without a thrown promise.
type ListRunningSessionsResult struct {
	Sessions []RunningSession `json:"sessions"`
	Error    string           `json:"error,omitempty"`
}

// ListRunningSessions returns the LIVE session registry for the named
// universe — every session the core's Room currently holds (running +
// recently-finished), across ALL runs on that core. This is the session
// manager service's view (Room.ListSessions over the Terminal slot), NOT
// the on-disk journal list ListSessions returns.
//
// Wails-bound: window.go.main.App.ListRunningSessions(universeName).
// The frontend calls this on mount + refreshes it on each `session.created`
// Wails event so the running-sessions view auto-updates as runs start.
func (a *App) ListRunningSessions(universeName string) ListRunningSessionsResult {
	a.mu.Lock()
	master := a.master
	ctx := a.ctx
	a.mu.Unlock()

	if master == nil || !master.IsActive(universeName) {
		// Universe not active → no live sessions (not an error).
		return ListRunningSessionsResult{Sessions: []RunningSession{}}
	}

	core, err := master.Activate(ctx, universeName) // idempotent fast-path when already active
	if err != nil {
		return ListRunningSessionsResult{Error: err.Error()}
	}

	infos, err := core.TerminalRouter().ListSessions(ctx)
	if err != nil {
		return ListRunningSessionsResult{Error: err.Error()}
	}

	out := ListRunningSessionsResult{Sessions: make([]RunningSession, 0, len(infos))}
	for _, s := range infos {
		out.Sessions = append(out.Sessions, RunningSession(s))
	}
	return out
}
