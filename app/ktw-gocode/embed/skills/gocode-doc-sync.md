---
name: gocode-doc-sync
description: >-
  How the documenter persona keeps documentation aligned with code
  reality: authored charter blocks (concepts_md / api_contract_md),
  godoc on exports, and the discipline of NEVER hand-editing projected
  charter sections.
icon: "📖"
iconName: book-open
hashtags: [documenter, doc, charter, godoc, ktw-gocode]
category: runbook
provides: [skill]
---

# Doc Sync — keep docs aligned with code

The documenter's job is to make sure that the documentation
reflects what the code actually does. Three surfaces matter:

1. **Charter authored blocks** (`charter.concepts_md`,
   `charter.api_contract_md`) — prose embedded as object fields
2. **godoc on exported Go symbols** — written next to the code
3. **`doc_folder`** (when the module has one) — long-form prose

## Authored vs projected — the rule that matters most

`charter.md` on disk has BOTH authored prose AND projected
sections. The boundary:

| Section | Source |
|---|---|
| Title + Meta | Projected (module fields) |
| Concepts & Glossary | **AUTHORED** in `charter.concepts_md` |
| Use Cases table | Projected (use-case objects) |
| Definition of Done — per UC | Projected (use-case bodies) |
| Open Decisions | Projected (`decision` objects in state `open`) |
| Public API Contract | **AUTHORED** in `charter.api_contract_md` |
| Audit History | Projected (the audit's review + its findings) |
| Manifest Stats Snapshot | Projected (counters) |

**Hard rule** (concepts.md §15.4): **NEVER hand-edit projected
sections in charter.md on disk.** The drift detector flags
hand-edits. To change projected content, change the underlying
object (UC body, OD title, a finding's state, etc.) and re-run
`charter-publish` (code service).

To change authored content:

1. Update the charter object's `concepts_md` or `api_contract_md`
   field via `cwm-object-update` (`cwm-object-get` it first, change
   the field, send the whole object back with the `revision` it gave)
2. Re-run `charter-publish` (code service) to project the new prose

## What you produce

- **Updates to `charter.concepts_md`** — when public API
  changes warrant new glossary terms or rephrased concepts
- **Updates to `charter.api_contract_md`** — when public symbols
  are added, removed, renamed, or change stability tier
- **godoc comments on exported symbols** — but only when missing
  or stale. Don't rewrite working godoc.
- **`charter-publish (code service)` invocations** — to regenerate the
  on-disk markdown after authored-prose changes

## What you do NOT do

- ❌ Hand-edit `charter.md` on disk in projected sections
- ❌ Author UC bodies (that's the specifier's job)
- ❌ Write decisions (that's the architect's job)
- ❌ Run `module.verify` / `module.audit` / `module.promote`
  (tester / reviewer / project-manager territories)

## godoc patterns

When a public Go symbol is missing godoc, add it following the
established voice of the package:

```go
// SpawnerLocal manages the lifecycle of child executors spawned
// by a parent workflow. Children share the parent's Channel for
// event routing; cancellation cascades via the per-runID context.
//
// Production usage spawns via Spawn(ctx, ChildSpec); tests can
// inject a custom Channel via NewLocalSpawnerWithChannel.
type SpawnerLocal struct {
    ...
}
```

**Conventions**:

- First sentence is a one-line summary; ends in a period
- Subsequent paragraphs cover usage / invariants / failure modes
- Reference companion types via their canonical name (no Go
  prose mangling — write "Channel" not "channels-package")
- Cite a Go test name for non-obvious invariants:
  "Concurrent Spawn calls share inbox dispatch — see
  TestSpawn_ConcurrentDispatch."

## Concepts/Glossary section — what to author

The Concepts block should answer: "what does an outside reader
need to know to navigate this module?"

Include:

- Domain terms used in UC titles / decision bodies that aren't
  Go stdlib / common terminology
- The module's core abstractions (Machine, Channel, Executor,
  etc.)
- Cross-references to companion modules (e.g. "tasks consumes the
  CWM substrate via internal/extensions/cwm/api/")

Don't include:

- Implementation details (those live in code comments)
- Stale content from a previous tier (purge when the abstraction
  evolves)
- TODO markers (file a defect item instead)

## API Contract section — what to author

The API Contract block should declare:

- **Public surface inventory** — exported types, functions, vars
  the module commits to (mirrors `code-symbol` items when those
  ship in Phase F)
- **Stability claims** per symbol — `stable` / `provisional` /
  `deprecated`. The promote gate uses this to decide whether a
  charter-version bump is needed.
- **Breaking-change policy** — when can `stable` symbols be
  removed? At a major charter-version bump.

## When you're done (default_gate: docs-current)

For the module you were spawned on:

- [ ] Every term used in UC titles / decision bodies has a
      glossary entry OR is universally understood
- [ ] Every exported Go symbol has godoc
- [ ] `charter.api_contract_md` matches the current public
      surface (no claimed-but-removed symbols, no missing
      newly-added symbols)
- [ ] `charter-publish (code service)` has been invoked after your
      authored-prose changes
- [ ] No drift detector findings on `charter.md`

## References

- Concepts.md §6.5 — charter item schema (concepts_md, api_contract_md)
- Concepts.md §15.4 — projected-vs-authored boundary; drift detector
- `projection/charter_build.go` — what sections are projected
