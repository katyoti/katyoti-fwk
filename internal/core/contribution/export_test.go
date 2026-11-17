// SPDX-License-Identifier: Apache-2.0

package contribution

// FieldPaths exposes fieldPaths to the external test package, whose tests
// import packs that import this package (an internal test would cycle).
var FieldPaths = fieldPaths
