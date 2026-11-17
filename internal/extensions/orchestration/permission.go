// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"fmt"
	"sync"
	"time"
)

// SwarmPermissionRequest tracks a permission request from a worker to a leader.
type SwarmPermissionRequest struct {
	ID         string                `json:"id"`
	WorkerID   string                `json:"workerId"`
	WorkerName string                `json:"workerName"`
	ToolName   string                `json:"toolName"`
	Input      map[string]any        `json:"input,omitempty"`
	Status     PermissionStatus      `json:"status"` // pending | resolved
	Resolution *PermissionResolution `json:"resolution,omitempty"`
	CreatedAt  time.Time             `json:"createdAt"`
}

// PermissionStatus is the state of a permission request.
type PermissionStatus string

const (
	PermissionPending  PermissionStatus = "pending"
	PermissionResolved PermissionStatus = "resolved"
)

// PermissionResolution is the leader's decision on a permission request.
type PermissionResolution struct {
	Decision     string         `json:"decision"` // "approve" | "deny"
	ResolvedBy   string         `json:"resolvedBy"`
	Feedback     string         `json:"feedback,omitempty"`
	UpdatedInput map[string]any `json:"updatedInput,omitempty"`
	ResolvedAt   time.Time      `json:"resolvedAt"`
}

// PermissionManager manages permission delegation between agents.
type PermissionManager struct {
	mu       sync.RWMutex
	requests map[string]*SwarmPermissionRequest // id → request
	nextID   int
}

// NewPermissionManager creates a permission manager.
func NewPermissionManager() *PermissionManager {
	return &PermissionManager{
		requests: make(map[string]*SwarmPermissionRequest),
	}
}

// CreateRequest creates a new pending permission request.
func (pm *PermissionManager) CreateRequest(workerID, workerName, toolName string, input map[string]any) string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.nextID++
	id := fmt.Sprintf("perm-%d", pm.nextID)

	pm.requests[id] = &SwarmPermissionRequest{
		ID:         id,
		WorkerID:   workerID,
		WorkerName: workerName,
		ToolName:   toolName,
		Input:      input,
		Status:     PermissionPending,
		CreatedAt:  time.Now(),
	}

	return id
}

// ResolveRequest resolves a pending permission request.
func (pm *PermissionManager) ResolveRequest(id, decision, resolvedBy, feedback string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	req, ok := pm.requests[id]
	if !ok {
		return fmt.Errorf("permission request %q not found", id)
	}
	if req.Status != PermissionPending {
		return fmt.Errorf("permission request %q already resolved", id)
	}

	req.Status = PermissionResolved
	req.Resolution = &PermissionResolution{
		Decision:   decision,
		ResolvedBy: resolvedBy,
		Feedback:   feedback,
		ResolvedAt: time.Now(),
	}
	return nil
}

// PollForResponse checks if a permission request has been resolved.
// Returns the resolution if resolved, nil if still pending.
func (pm *PermissionManager) PollForResponse(id string) (*PermissionResolution, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	req, ok := pm.requests[id]
	if !ok {
		return nil, fmt.Errorf("permission request %q not found", id)
	}
	if req.Status == PermissionPending {
		return nil, nil
	}
	return req.Resolution, nil
}

// ListPending returns all pending permission requests.
func (pm *PermissionManager) ListPending() []*SwarmPermissionRequest {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var pending []*SwarmPermissionRequest
	for _, req := range pm.requests {
		if req.Status == PermissionPending {
			pending = append(pending, req)
		}
	}
	return pending
}

// CleanupOldResolutions removes resolved requests older than the given duration.
func (pm *PermissionManager) CleanupOldResolutions(maxAge time.Duration) int {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for id, req := range pm.requests {
		if req.Status == PermissionResolved && req.Resolution.ResolvedAt.Before(cutoff) {
			delete(pm.requests, id)
			removed++
		}
	}
	return removed
}
