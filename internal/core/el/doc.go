// SPDX-License-Identifier: Apache-2.0

// Package el provides the project-wide expression-language utilities built
// on github.com/expr-lang/expr.
//
// All consumers (workflow YAML evaluator, task filter predicate, the drawio
// template bindings and ktw-when conditions) share one Cache type,
// one compile-error wrapping convention, one Truthy contract, and one
// wrapped-expression syntax (`${...}`) — the only way to opt a string
// into evaluation.
//
// User-facing reference: modules/framework/el/concepts.md.
package el
