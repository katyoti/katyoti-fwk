// SPDX-License-Identifier: Apache-2.0

package em

import "fmt"

// ErrDuplicatePointID is returned when registering a point with an ID or alias
// that already exists.
type ErrDuplicatePointID struct {
	ID string
}

func (e ErrDuplicatePointID) Error() string {
	return fmt.Sprintf("em: duplicate extension point ID or alias: %q", e.ID)
}

// ErrPointNotFound is returned when contributing to or looking up a point
// that does not exist.
type ErrPointNotFound struct {
	ID string
}

func (e ErrPointNotFound) Error() string {
	return fmt.Sprintf("em: extension point not found: %q", e.ID)
}

// ErrDuplicateKey is returned when an IndexedPoint receives a contribution
// with a key that already exists and AllowReplace is false.
type ErrDuplicateKey struct {
	PointID string
	Key     string
}

func (e ErrDuplicateKey) Error() string {
	return fmt.Sprintf("em: duplicate key %q in extension point %q", e.Key, e.PointID)
}

// ErrTypeMismatch is returned when GetPoint is called with a type parameter
// that does not match the registered point's type.
type ErrTypeMismatch struct {
	PointID  string
	Expected string
	Got      string
}

func (e ErrTypeMismatch) Error() string {
	return fmt.Sprintf("em: type mismatch for point %q: expected %s, got %s", e.PointID, e.Expected, e.Got)
}
