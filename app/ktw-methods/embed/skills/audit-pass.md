---
name: audit-pass
description: >-
  Run a perspective-based audit — sweep an artifact once per lens (architecture,
  security, performance, dead-code, correctness, …) and emit structured,
  evidence-backed findings, each severity-tagged. Use when asked to review /
  audit / find issues across dimensions. Pairs with the reviewer persona. NOT a
  completeness check (that's completeness-gate, which is computed).
icon: "🔍"
iconName: zoom-scan
hashtags: [audit, review, findings, perspective, security, architecture]
category: lifecycle
---

# Audit pass

Audit by **lens**, not by vibe. A single read looking for "anything wrong" misses
most things; one focused sweep per perspective catches what that lens is tuned
for. The output is a list of **findings**, each backed by evidence and tagged by
severity — never a prose "looks fine".

## The lenses

Pick the lenses that fit the artifact (code, a dossier, a schema, a design):

| Lens | Asks |
|---|---|
| **architecture** | boundaries, coupling, layering, duplication, the wrong thing in the wrong place |
| **security** | trust boundaries, input validation, secrets, authz, injection surfaces |
| **performance** | hot paths, N+1 / unbounded loops, allocation, blocking calls, missing backoff |
| **dead-code** | unreachable / unused / superseded code + parallel artifacts that drifted |
| **correctness** | edge cases, error paths, off-by-one, contract violations vs the ISA |
| **consistency** | naming, conventions, the pack's own rules + decisions |

Run each lens as its OWN sweep — don't blend them; a blended pass reverts to vibe.

## Finding shape

Every finding is one record:

```
- [<severity>] <lens> — <one-line problem>
  where: <file:line / node / section>
  evidence: <the concrete thing that shows it — quoted, not described>
  fix: <the proposed change, WITHOUT applying it>
```

`severity ∈ {critical, high, medium, low}`. **Evidence is mandatory** — a finding
you can't point at is a hunch; drop it or go find the evidence. No speculative
findings.

## Procedure

1. **Scope + pick lenses.** State what's in scope and which lenses apply (and
   which you're skipping + why).
2. **One sweep per lens.** For each, read with only that lens's question in mind;
   emit findings as you go, each with where + evidence.
3. **Dedup + rank.** Merge findings that are the same root cause; sort by
   severity, then by blast radius.
4. **Conclude (ISA-style).** End with exactly one of: **NOTHING FOUND** (the
   artifact passes every applied lens — say it plainly), or the **ranked findings
   list**. No vague middle. (Same binary-conclusion discipline as the eval skills.)
5. **Hand off — don't auto-fix.** Present findings; the owner decides which to
   apply (and a non-obvious accept/reject is a `ktw-methods:/skill/decision-capture`).

## Anti-criteria

- **Anti:** a finding with no `where` + `evidence` (a hunch, not a finding).
- **Anti:** a single blended "review" pass instead of one sweep per lens.
- **Anti:** concluding with prose ("mostly good") instead of NOTHING FOUND or a
  ranked list.
- **Anti:** applying fixes during the audit — audit emits findings; fixing is a
  separate, owner-gated step.
