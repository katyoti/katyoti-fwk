// SPDX-License-Identifier: Apache-2.0

package journal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// DelegationNode represents one agent run in the delegation tree.
// Built from session_start events across all session journals in a directory.
type DelegationNode struct {
	// RunID is the unique run identifier (matches the executor's run id,
	// stored in session_start event's emitter field as "run-{id}").
	RunID string `json:"runId"`

	// SessionID is the on-disk session directory name (e.g., "2026-04-07T08-02-31_goal-planner").
	SessionID string `json:"sessionId"`

	// Dir is the absolute path to the session directory.
	Dir string `json:"dir"`

	// Task is the task name from the session_start event.
	Task string `json:"task"`

	// Mode is the resolved delegation mode (worker, coordinator, teammate, etc.).
	// Empty for sessions that don't declare a mode.
	Mode string `json:"mode,omitempty"`

	// Model is the LLM model name used for this run.
	Model string `json:"model,omitempty"`

	// Provider is the LLM provider name.
	Provider string `json:"provider,omitempty"`

	// ParentRunID is the run id of the parent that spawned this run.
	// Empty for top-level runs (roots of the tree).
	ParentRunID string `json:"parentRunId,omitempty"`

	// StartedAt is the session start timestamp (RFC3339).
	StartedAt string `json:"startedAt"`

	// Children are child nodes spawned by this run via delegate or spawn_teammate.
	// Sorted by start time (oldest first).
	Children []*DelegationNode `json:"children,omitempty"`
}

// sessionStartEvent matches the structure of a session_start journal entry.
// Mirrors api.SessionStartEvent but kept local to avoid coupling ktw-utils
// to the agent api package.
type sessionStartEvent struct {
	Type string `json:"type"`
	Data struct {
		Type        string `json:"type"`
		Time        string `json:"ts"`
		Emitter     string `json:"emitter"`
		Name        string `json:"name"`
		Mode        string `json:"mode"`
		ParentRunID string `json:"parentRunId"`
		Model       string `json:"model"`
		Provider    string `json:"provider"`
	} `json:"data"`
}

// readSessionStart reads the first line of llm.jsonl and parses it as a
// session_start event. Returns nil if the file is missing, empty, or the
// first event is not a session_start.
func readSessionStart(sessionDir string) (*DelegationNode, error) {
	llmPath := filepath.Join(sessionDir, "llm.jsonl")
	f, err := os.Open(llmPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty journal: %s", llmPath)
	}

	var evt sessionStartEvent
	if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
		return nil, fmt.Errorf("parse session_start: %w", err)
	}
	if evt.Type != "session_start" {
		return nil, fmt.Errorf("first entry is not session_start: %s", evt.Type)
	}

	return &DelegationNode{
		RunID:       evt.Data.Emitter,
		Dir:         sessionDir,
		Task:        evt.Data.Name,
		Mode:        evt.Data.Mode,
		Model:       evt.Data.Model,
		Provider:    evt.Data.Provider,
		ParentRunID: evt.Data.ParentRunID,
		StartedAt:   evt.Data.Time,
	}, nil
}

// BuildDelegationTree scans all sessions in sessionsDir, reads each one's
// session_start event, and builds the parent → children tree rooted at the
// session matching rootSessionID.
//
// If rootSessionID is empty, it returns the most recent root (a session with
// no parentRunId). If rootSessionID is given, it locates that session and
// builds the tree from there.
//
// Sessions older than the root are ignored. Sessions whose parent is not in
// the tree are skipped (orphans).
func BuildDelegationTree(sessionsDir, rootSessionID string) (*DelegationNode, error) {
	sessions, err := ListSessions(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found in %s", sessionsDir)
	}

	// Read session_start for each session, building a map from runID → node.
	byRunID := make(map[string]*DelegationNode)
	bySessionID := make(map[string]*DelegationNode)
	var nodes []*DelegationNode
	for _, s := range sessions {
		node, err := readSessionStart(s.Dir)
		if err != nil {
			// Skip unreadable sessions silently — likely in-progress or corrupt
			continue
		}
		node.SessionID = s.ID
		byRunID[node.RunID] = node
		bySessionID[s.ID] = node
		nodes = append(nodes, node)
	}

	if len(byRunID) == 0 {
		return nil, fmt.Errorf("no readable sessions in %s", sessionsDir)
	}

	// Find the root node
	var root *DelegationNode
	if rootSessionID != "" {
		root = bySessionID[rootSessionID]
		if root == nil {
			// Try suffix match (allow short ids)
			for sid, node := range bySessionID {
				if sid == rootSessionID || node.RunID == rootSessionID {
					root = node
					break
				}
			}
		}
		if root == nil {
			return nil, fmt.Errorf("session not found: %s", rootSessionID)
		}
	} else {
		// Most recent session with no parent (sessions list is newest-first)
		for _, node := range nodes {
			if node.ParentRunID == "" {
				root = node
				break
			}
		}
		if root == nil {
			return nil, fmt.Errorf("no top-level sessions found")
		}
	}

	// Build children index: parent runID → []*DelegationNode
	childrenByParent := make(map[string][]*DelegationNode)
	for _, node := range nodes {
		if node.ParentRunID != "" {
			childrenByParent[node.ParentRunID] = append(childrenByParent[node.ParentRunID], node)
		}
	}

	// Recursively attach children, traversing only descendants of root.
	attachChildren(root, childrenByParent)
	return root, nil
}

func attachChildren(node *DelegationNode, childrenByParent map[string][]*DelegationNode) {
	children := childrenByParent[node.RunID]
	// Sort children by start time so they appear in invocation order
	sort.Slice(children, func(i, j int) bool {
		return children[i].StartedAt < children[j].StartedAt
	})
	for _, child := range children {
		attachChildren(child, childrenByParent)
		node.Children = append(node.Children, child)
	}
}
