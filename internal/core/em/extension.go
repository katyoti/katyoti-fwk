// SPDX-License-Identifier: Apache-2.0

// Package em provides the Extension Manager — a typed extension point registry.
//
// The EM has no domain imports (stdlib + sync only). All domain types live in
// extension packages. Phase management and hook orchestration are handled by
// the application package.
package em

// ExtensionPoint is a typed slot that accepts contributions of type T.
// The EM routes contributions to matching points. Points own their data.
// The point ID is derived automatically from the Go type T via PointID[T]().
type ExtensionPoint[T any] interface {
	// Contribute adds an item to this extension point.
	Contribute(item T) error

	// Entries returns all contributed items.
	Entries() []T
}
