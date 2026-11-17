// SPDX-License-Identifier: Apache-2.0

// Package api defines the public interface and DTOs for ktw-gocode —
// the Go-coding method pack on top of CWM.
//
// This package has zero dependencies on internal/* packages beyond
// internal/core/meta and internal/core/reflect. Consumers (the
// sidecar binary at app/ktw-gocode/ once split out, the CLI surface,
// agent tool wrappers, tests, other method packs) import it freely.
//
// Layered architecture:
//
//	api/             ← THIS PACKAGE: types + interface + ServiceDef
//	local/           ← Extension() wiring (publish, KTSP, agent-tool, HTTP)
//	internal/...     ← service implementation (projection, verbs,
//	                   discovery, tier FSM, …)
//
// # Two services, one package
//
// ktw-gocode publishes TWO service interfaces (both via AutoKTSPContainer):
//
//   - [GocodeService] — orchestration + items + tasktest verbs (~19)
//   - [GoToolService] — pure Go toolchain wrappers (~10)
//
// They live in the same package because they share entity types
// (Module, UseCase, TestCase, …) and ship from the same binary.
// `AutoKTSPContainer` registers each under its own slot namespace
// (`gocode.*` for orchestration, `gotool.*` for toolchain).
//
// # Interface organization
//
// Both services are FLAT: methods named <Slot><Verb> (e.g. ModuleVerify,
// UCSetStatus, GoTest). Slot grouping happens at the meta.ServiceDef.Methods
// map keys ("module.Verify", "uc.SetStatus", "go.Test") for CLI auto-
// generation.
//
// All methods follow the context-first signature:
//
//	Method(ctx context.Context, in InputType) (OutputType, error)
//
// This matches reflect.DiscoverMethods conventions; AutoKTSPContainer
// auto-discovers them at AppInit.
//
// # Entities vs Verbs
//
// Entity structs (Module, UseCase, OpenDecision, TestCase, TestRun,
// AuditBundle, AuditFinding, Feature) define the on-the-wire shape
// of items + nodes for serialization. They mirror the CWM YAML
// item-type / node-type schemas defined under
// app/ktw-agents/embed/cwm/{item_types,node_types}/.
//
// Verb DTOs (ModuleVerifyInput, UCCreateInput, …) define the
// argument and return shapes for each method.
package api
