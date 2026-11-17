// SPDX-License-Identifier: Apache-2.0

package api

// D6 — typed identifier string types. Defined here for adoption by future
// code (D5 unify-run-lifecycle-types is the natural migration point for
// the existing runEntry / bgEntry / ChildHandle / TaskRecord fields). The
// goal is to make a compile-time error of passing a ChildID where a RunID
// is expected — the historic "every ID is a bare string" footgun caught
// nobody during code review.
//
// **Migration discipline**: new public signatures use these typed values;
// existing string-typed fields stay as `string` until D5 lands. The types
// are byte-compatible with `string` (named-string types underneath), so
// JSON/YAML serialisation is unchanged and the migration is mechanical.

// RunID identifies a single execution of any Machine (workflow, agent,
// simulator, ...). Stamped at executor.Run time; used to scope events on
// the bus and to address an Inbox via Channel.Wait.
type RunID string

// TaskID identifies a TaskRecord in the BackgroundTaskManager — distinct
// from RunID because background tasks have a lifecycle layer above the
// executor (notification queue, kill semantics, parent waiting). The
// underlying value happens to equal the RunID of the wrapped run today,
// but the type distinction makes future divergence (e.g. retried task =
// new RunID, same TaskID) cheap to model.
type TaskID string

// ChildID identifies a single child spawn within a parent workflow's
// LocalSpawner.children map. Distinct from RunID because the parent
// tracks children by handle name + RunID; the ChildID embeds both
// concepts.
type ChildID string

// NodeID identifies a workflow Node within a workflow's activation stack.
// String-shaped because the loader generates them as `<prefix>_<seq>`
// (see internal/wkflow/loader.go nextNodeID). Pure intra-workflow scope.
type NodeID string
