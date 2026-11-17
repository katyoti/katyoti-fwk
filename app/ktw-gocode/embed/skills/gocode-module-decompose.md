---
name: gocode-module-decompose
description: >-
  Decision tree for the architect persona: when to split code into a
  new module vs extend an existing one, how to pick the kind
  (framework / library / sidecar), and how to lay out the public /
  internal / local Go package triple.
icon: "📐"
iconName: layers
hashtags: [module, decompose, architect, kind, layout, ktw-gocode]
category: reference
provides: [reference]
---

# Module Decomposition

This skill is the architect's reference for placement decisions:
new module vs extend, which kind, and what the Go layout looks like.

## Step 1 — Do we need a new module?

Default to **extending an existing module**. Create a new one only
when one of these holds:

- The code provides a **distinct logical capability** that other
  modules consume independently (not just a helper)
- The code has **independent quality lifecycle** (will be tier-
  promoted on its own schedule, not in lockstep with the parent)
- The code has **substantially different rules** (e.g. stdlib-only
  vs. third-party-allowed) that lint must apply differently
- The code is large enough that its UC inventory would dominate
  the parent module's charter

Cheap-rule heuristic: if you'd describe the code as "the X package
inside Y" → extend Y. If you'd describe it as "the X module" →
new module.

## Step 2 — Pick the kind

Exactly one of:

### `framework`

The module **provides extension points** that other modules
register against. It is the host of an EM (extension mechanism)
point. Examples in this repo: `internal/core/application/`,
`internal/kts/`.

Rules:
- STRICT: stdlib + intra-framework imports only
- Public surface tends to be small (interface declarations +
  registration helpers); internal/ holds the dispatch machinery
- Frameworks rarely get tier-promoted aggressively — stability
  matters more than feature growth

### `library`

The module **exports a typed API** consumed by other modules via
import paths. Examples: `internal/extensions/cwm/api/`,
`internal/extensions/task/api/`.

Rules:
- Public surface in `public/` (or top-level `api/` per repo
  convention); implementation in `internal/`
- `local/` holds the `application.Extension` factory that wires
  the library into a binary
- Library modules can be tier-promoted independently when their
  feature set warrants

### `sidecar`

The module **produces an executable** under `app/<name>/`.
Examples: ktw-docs, ktw-gocode itself.

Rules:
- MUST have an `app/<name>/cmd/{root,bootstrap}.go` shell + a
  buildable `main.go`
- The sidecar binary publishes one or more services via
  `appcore.PublishService`
- Public surface aims at the spawning hub (KTSP slot methods,
  HTTP routes, CLI verbs auto-generated via AutoKTSPContainer)

## Step 3 — The 3-package layout

Every module follows this triple (concepts.md §5.2):

```
<module-root>/
├── public/      # exported API; godoc on every export
├── internal/    # private implementation; intra-module imports
└── local/       # framework contributions (application.Extension factories)
```

**Import rules**:

- `public/` may import: stdlib + same-module-public + cross-module-public
- `internal/` may import: stdlib + anything within the same module
  + cross-module-public (but NOT cross-module-internal)
- `local/` may import: everything within the same module (its job
  is to wire pieces); cross-module imports are allowed but
  discouraged — local should mainly compose what's local

Some legacy modules use top-level `api/` instead of `public/`;
that's the convention in this repo. Treat `api/` as the public
surface when present, equivalent to `public/`.

## Step 4 — The `realized-by` edges

After deciding placement, record the souhait→livraison join:

- Each `feature` that this module realizes gets a `realized-by`
  edge pointing at this module's node id
- A module may be on the receiving end of multiple `realized-by`
  edges (one feature can require multiple modules; one module
  can realize multiple features — that's the M:N edge shape)

The aggregate `realization-status` derived field rolls up the
linked modules' tier_state + UC verdicts (concepts.md §7.1).
Updating module tier flips automatically updates feature
status — no hand-maintained checkbox.

## Worked example — adding a new sidecar

The user describes a feature: "we need to expose CWM via HTTP for
external dashboards."

1. **New module or extend?** New. The HTTP exposure is a distinct
   deliverable (a separate binary) with its own quality lifecycle.
2. **Kind?** Sidecar — it produces a binary under
   `app/ktw-cwm-http/`.
3. **Layout?** `app/ktw-cwm-http/{main.go, embed.go, cmd/...}` +
   `internal/extensions/cwmhttp/{api,internal,local}/`.
4. **`realized-by` edge** from the feature node to the new
   `ktw-cwm-http` module node.
5. **Decision items** documenting: why a separate sidecar vs
   adding HTTP to cwm-local directly; what port; auth model.

## When in doubt

Record the question as a **`decision` in state `open`** (an OD,
`ktw-methods:decisions/Open`) rather than guessing. The verify gate is forgiving of open ODs at tier 0; it's
unforgiving of bad placement decisions baked into committed code.

## References

- Concepts.md §5 — module node-type schema (kind / facets / tier)
- `app/ktw-docs/`, `app/ktw-gocode/` — two worked sidecar
  examples to mirror
