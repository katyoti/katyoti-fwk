// SPDX-License-Identifier: Apache-2.0

package em

import (
	"fmt"
	"log/slog"
)

// EM is the Extension Manager — a typed extension point registry.
// It holds registered extension points and tracks ownership/contributions.
// Phase management and hook orchestration are handled by the application package.
type EM struct {
	points       map[string]any      // type-erased ExtensionPoint[T] values
	pointOwners  map[string]string   // point ID → registering extension name
	contributors map[string][]string // point ID → extension names that contributed
	currentExt   string              // name of extension currently being processed
	logger       *slog.Logger
}

// New creates a new Extension Manager.
func New() *EM {
	return &EM{
		points:       make(map[string]any),
		pointOwners:  make(map[string]string),
		contributors: make(map[string][]string),
		logger:       slog.Default(),
	}
}

// SetCurrentExt sets the name of the extension currently being processed.
// Called by Application during hook execution for ownership tracking.
func (em *EM) SetCurrentExt(name string) { em.currentExt = name }

// SetLogger sets a custom logger for the EM.
func (em *EM) SetLogger(logger *slog.Logger) {
	em.logger = logger
}

// PointInfo holds metadata about a registered extension point.
type PointInfo struct {
	ID           string   // point ID (derived from Go type via PointID[T]())
	Owner        string   // name of the extension that registered this point
	Contributors []string // names of extensions that contributed (one entry per contribution)
}

// PointInfo returns metadata about a registered extension point.
// Returns nil if the point is not found.
func (em *EM) PointInfo(pointID string) *PointInfo {
	if _, ok := em.points[pointID]; !ok {
		return nil
	}

	info := &PointInfo{
		ID:    pointID,
		Owner: em.pointOwners[pointID],
	}

	if c := em.contributors[pointID]; len(c) > 0 {
		info.Contributors = make([]string, len(c))
		copy(info.Contributors, c)
	}

	return info
}

// PointsInfo returns metadata about all registered extension points.
func (em *EM) PointsInfo() []PointInfo {
	infos := make([]PointInfo, 0, len(em.points))
	for id := range em.points {
		if pi := em.PointInfo(id); pi != nil {
			infos = append(infos, *pi)
		}
	}
	return infos
}

// RegisterPoint registers a typed extension point with the EM.
// The point ID is derived automatically from type T via PointID[T]().
// Phase gates are enforced by the application package, not here.
//
// This is a free function because Go methods cannot have type parameters.
func RegisterPoint[T any](em *EM, point ExtensionPoint[T]) error {
	id := PointID[T]()

	if _, exists := em.points[id]; exists {
		return ErrDuplicatePointID{ID: id}
	}

	em.points[id] = point

	if em.currentExt != "" {
		em.pointOwners[id] = em.currentExt
	}

	em.logger.Debug("RegisterPoint", "id", id)
	return nil
}

// Contribute adds an item to a registered extension point.
// The point is looked up by PointID[T]().
// Phase gates are enforced by the application package, not here.
func Contribute[T any](em *EM, item T) error {
	id := PointID[T]()
	raw, ok := em.points[id]
	if !ok {
		return ErrPointNotFound{ID: id}
	}

	point, ok := raw.(ExtensionPoint[T])
	if !ok {
		return ErrTypeMismatch{
			PointID:  id,
			Expected: fmt.Sprintf("ExtensionPoint[%T]", *new(T)),
			Got:      fmt.Sprintf("%T", raw),
		}
	}

	if err := point.Contribute(item); err != nil {
		return err
	}

	if em.currentExt != "" {
		em.contributors[id] = append(em.contributors[id], em.currentExt)
	}

	return nil
}

// GetPoint returns a typed extension point by its auto-derived ID.
func GetPoint[T any](em *EM) (ExtensionPoint[T], error) {
	id := PointID[T]()
	raw, ok := em.points[id]
	if !ok {
		return nil, ErrPointNotFound{ID: id}
	}

	point, ok := raw.(ExtensionPoint[T])
	if !ok {
		return nil, ErrTypeMismatch{
			PointID:  id,
			Expected: fmt.Sprintf("ExtensionPoint[%T]", *new(T)),
			Got:      fmt.Sprintf("%T", raw),
		}
	}

	return point, nil
}
