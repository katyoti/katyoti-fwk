// SPDX-License-Identifier: Apache-2.0

package local

// points.go — the manifest extension's EM extension points.
//
// Declared HERE, not in internal/core. An extension point is WIRING: this
// package is the only thing that registers one, and the domain layer reads
// their contributions type-routed through application.UseExtensions, never by
// naming the point.
//
// Two points, registered by Extension() — a FRESH point per application:
//
//   - ContributionProvider: every binary that ships skills, references, tasks,
//     scripts, capabilities or any custom resource type adds a provider;
//     aggregating consumers (the descriptor builder, the catalog) enumerate
//     them. It replaces the per-type loader EPs from the pre-NATS era.
//   - mbapi.RefreshHook: the contribution-change subscriber refreshes the
//     in-process catalog index, then fans out to every hook whose Kinds match
//     (SkillManager, the CWM type store, …).
//
// Never a package-level point instance: a list point is state, and one shared
// instance registered into every application of a process made the second
// application enumerate the first one's providers (two CWM test applications
// in one test binary served the first one's types).
