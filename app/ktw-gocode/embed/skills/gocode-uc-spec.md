---
name: gocode-uc-spec
description: >-
  Conventions for authoring use-case (UC) items: the UC-NNN id format,
  severity classification, DoD body structure with layer-tagged test
  refs, and the negative-DoD requirement for security UCs. Reference
  for specifier (creating UCs) and coder (reading them).
icon: "📝"
iconName: list-checks
hashtags: [uc, use-case, dod, ktw-gocode, conventions]
category: reference
provides: [reference]
---

# Use-Case (UC) Authoring Conventions

This skill defines how to write a `use-case` item that the
ktw-gocode verify aggregator can consume. Concepts.md §6.4 +
coverage/matcher.go are the source of truth; this is the
operator-facing cheat sheet.

## The UC ID format

Stable, gap-allowed, **always 3-digit-padded**:

```
UC-001    UC-053    UC-150    UC-1000
```

**Rules:**

- Allocate sequentially; **leave gaps** between thematic groups
  (UC-001..UC-019 for engine primitives, UC-020..UC-035 for
  workflow interpreter, etc.) so future expansion never requires
  renumbering.
- Once allocated, an id is **immutable** — never re-number a UC
  after items reference it.
- `fail-deferred` UCs keep their id; the OD link records the
  rationale.

## Severity — pick exactly one

| Severity | Use when… | Negative DoD required? |
|---|---|---|
| `core` | module is broken without it (engine primitives, integration backbones) | optional |
| `security` | safety / authorization / sanitization invariants | **YES** |
| `qol` | quality-of-life — better error messages, perf, ergonomics | no |
| `cosmetic` | naming, formatting, polish | no |

When unsure between `core` and `security`: ask "could a bypass of
this UC let an unauthorized caller do something?" If yes,
`security`. Otherwise `core`.

## DoD body structure

The use-case `body` field carries the DoD-per-UC. Format:

```markdown
**Trigger / setup**

(One-line description of the entry point. What's the call shape?
What state is in scope? Skip when self-evident from the title.)

**L1 evidence**

Go test name + assertion shape. Example:

- `internal/extensions/task/api/engine/executor_test.go::TestMultiStep_CounterToTarget`
- Constructs an Executor with a Passthrough machine, calls
  `Run(ctx, startEvent)`, asserts `RunResult.Next == Done` and
  `Output` equals expected.

**L2 evidence** _(when applicable)_

`.tasktest` scenario file + journal/trace assertion. Use simulator
unless the UC intrinsically needs an LLM.

- `tests/L2/uc-053-spawn-resume.tasktest` — spawns simulator child;
  asserts parent resumes with child's output bound into next step.

**L3 evidence** _(rare; cost-incurring)_

TASK.md scenario for cross-binary / orchestration UCs.

**Negative DoD** _(REQUIRED for severity: security)_

What MUST NOT happen. Explicit assertions of forbidden behavior.

- The spawn-permission-hook MUST deny `bash` when
  `DisallowedTools: ["bash"]` is set; assert journal does not
  contain a `bash` tool-call after the deny.
```

## The TCID matching convention

Test-case items reference their UC via the `tc_id` body field
following:

```
L<n>-uc<NNN>-<slug>
```

Examples:

```
L1-uc001-executor-multistep        ↔ UC-001
L2-uc053-spawn-resume              ↔ UC-053
L3-uc150-end-to-end-magentic       ↔ UC-150
```

The coverage aggregator parses this format to join tests to UCs;
non-conforming TCIDs surface as orphan test-case lint signals.
**Always** name test-cases following this pattern so verify can
aggregate.

## What NOT to put in a UC body

- ❌ Implementation hints (the coder decides)
- ❌ "We should add..." (UCs are present-tense observable
  invariants, not aspirations)
- ❌ Multi-UC fan-out ("This UC also requires UC-002 + UC-003 to
  pass") — file them as separate UCs and link via the DoD body
  ("preconditions" prose)
- ❌ Status fields hand-written (`status_cache` is verify's
  exclusive territory)

## When fail-deferring

When a UC is intentionally not implemented for a tier, open a
`decision` explaining why (`ktw-methods:decisions/Open`; it stays `open`), then link the UC to it with a
`deferred-by` edge — a relation is an edge, never a field. The verify
gate treats fail-deferred UCs as documented limitations (not
blockers), but only if the linked OD exists in state=open.

Example:

```yaml
# m.UC-073 fail-deferred against m.OD-004
edges:
  - kind: deferred-by
    to: m.OD-004
fields:
  status_cache: fail-deferred   # written by verify when it sees the link
```

## References

- Concepts.md §6.4 — full use-case item schema
- Concepts.md §9 — coverage state machine + 4-verdict projection
- `internal/extensions/gocode/internal/coverage/matcher.go` — the
  TCID parser the convention must satisfy
