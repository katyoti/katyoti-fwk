// SPDX-License-Identifier: Apache-2.0

package em

import "reflect"

// PointID returns the unique extension point identifier for type T.
// It uses the fully-qualified Go type name, ensuring each distinct type
// maps to exactly one extension point.
func PointID[T any]() string {
	return reflect.TypeFor[T]().String()
}
