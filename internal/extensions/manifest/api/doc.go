// SPDX-License-Identifier: Apache-2.0

// Package api is the consumer surface of the contribution system and the
// definition of its two wire services: the per-binary ResourceService (slot
// `resources`, the producer) and the hub's CatalogService (slot
// `catalog-service`, the aggregate). It also defines MessageBus
// (message_bus.go), the only access path to NATS for application code.
//
// This file OWNS the contribution vocabulary: which names are canonical, and
// which older names survive on the wire or in the API only for a caller that
// has not moved yet — with that caller named and the condition under which
// the name goes. Other documents point here (modules/contributions/tools.md,
// "Compatibility vocabulary"); they do not restate it. The operation-by-
// operation mapping, with band behaviour, is
// modules/contributions/contract-matrix.md.
//
// # Canonical surface
//
// Consumer calls (Go), one question each:
//
//   - Query (query.go) — the index: envelopes of one kind or all, at self or
//     universe reach. The only list surface.
//   - QueryTyped (query.go) — Query plus the data layer decoded into T.
//   - GetOne (query.go) — one entry by URI, data layer decoded, without
//     listing its kind.
//   - GetBody (CatalogReader.GetBody, catalog.go) — the `data` layer and
//     nothing else; empty, not an error, for a kind that has none.
//   - Open (layer.go) — any named layer (empty = the kind's default),
//     band-aware: inline and control bytes, bulk streamed. A layer the entry
//     does not declare is contribapi.ErrLayerNotDeclared at self reach and at
//     every in-process universe view (never another lane's answer); over the
//     wire the hub answers it as an invalid-request error.
//   - KindsAt / KindSchemas (query.go) — the kind registry of a view.
//
// Producer primitive: the provider's Open(id, layer), reached through
// ResourceService.OpenLayer.
//
// Wire, hub (CatalogService): GetManifest, ListNames, ListContributions (the
// index, always in index form), ListURIs, GetBody, OpenLayer. Wire, producer
// (ResourceService, `resources/*`): get-manifest, list, get, open-layer,
// types, get-data-type.
//
// # Compatibility names
//
// Every name below is live but not canonical. "Callers" is the in-tree
// census of 2026-09-25 (ms-contribution-ext e16); no polyglot client or
// producer calls any of them (ktw-display, ktw-ktsp-sdk: none). Policy is
// keep-until <the named consumer migrates> or remove-in <todo>; a name with
// no caller left is deleted, not kept "for safety". ListMeta is the model:
// the headers-only twin of ResourceService.List, removed 2026-06-05 when its
// last caller moved to List (WithBody false, the default) — its declaration
// had drifted from the live container, six methods advertised for five
// served — and the removal recorded here.
//
//	Name                                  Callers                                                        Policy
//	------------------------------------  -------------------------------------------------------------  ------------------------------------------------
//	CatalogService.ListResources          none in tree since 2026-09-25: manifestremote.Catalog.Index    compat: removed after the next tagged release
//	(wire catalog-service/list-resources, and every test moved to ListContributions (same code path    following 2026-09-25 (a client built before the
//	output key "resources")               in the hub, listPage). Kept only for a client built before     rename still pages through it), then delete with
//	                                      the rename.                                                    its Input/Output types.
//	CatalogListResourcesInput.IncludeBody no true-caller left: the catalog tests read bodies through     goes with ListResources (above). The new input
//	(wire field "includeBody")            GetBody/OpenLayer (2026-09-25).                                has no body switch: the index is always body-free.
//	CatalogService.Read                   manifest/remote/catalog.go (Catalog.Read) <- ktw-gui           keep-until ktw-gui app_workspace_capability.go
//	(wire catalog-service/read)           app_workspace_capability.go; the hub's own OpenLayer           opens the layer (mbapi.Open) AND the hub's
//	                                      re-enters it for a layer Read serves (ktw-hub/cmd              OpenLayer stops re-entering Read (e13 moves the
//	                                      catalog_service.go, readServes)                                producer route to resources/open-layer).
//	ResourceService.Read                  hub CatalogService.Read (ktw-hub/cmd catalog_service.go);      keep-until the hub route above is gone (e13); the
//	(wire resources/read)                 CLI `sc resources read` (manifest/commands/resources.go).      CLI verb then maps to open-layer.
//	                                      LLMService.ReadResource opens the layer since 2026-09-25.
//
// Removed in e13: the provider's pre-Open read path (its per-item getter, the
// body type it returned and the helper that decoded it) — the provider's
// Open(id, layer) through contribapi.OpenEnumerated is the only read path.
//
// Removed 2026-09-25 (e16), recorded here as ListMeta is: the legacy
// single-type filter CatalogListResourcesInput.Type (wire field "type"; no
// caller set it — a client still sending it is now unfiltered by it, use
// Filter.Types); the mbapi.KindDeclaration alias (use
// contribapi.KindDeclaration); and the MemoryCatalog's answer to an
// undeclared data/source layer through GetBody (bulk_lane.go), which served
// the data layer under the source layer's name. The GetResource catalog text
// left in ktw-gui app_ktsp_browser.go is a comment, owned by the docs pass.
//
// LLMService's ListResources / GetResource / ReadResource / ListResourceTypes
// (appcore/sidecar/api/llm.go) are the LLM-facing projection of a binary's
// own contributions, named for that audience; they are outside this
// vocabulary and are not listed as compatibility names.
package api
