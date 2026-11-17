# The create-method loop — in depth

The reference behind `ktw-methods:/skill/create-method`. The skill is the map
+ routing; this is the depth: each lens, the two principles, and a worked
example. It does NOT restate the routed skills — load those at their stage.

## The lenses, in depth

The four lenses are not phases — you hold all four at once, but the WORK moves
left-to-right through the loop.

- **WANT (the spec lens).** What does "done" mean, stated as claims that can
  fail? A want with no falsifiable criterion constrains nothing (ISA's
  hard-to-vary rule). The artifacts: a `feature` (what a test declares done)
  serving one or more `goal`s (directions, never done), the `use-case`s that
  prove it, and their `acceptance-criterion` objects (one binary probe each,
  ≥1 `anti`). Articulated via `isa`; the goal layer via `define-goals`; raw
  ideas shaped via `new-functionality`.
- **HAVE (the solution lens).** What already exists that this overlaps? The
  single most common mistake is adding a near-duplicate. `value-fit` triages
  each candidate against the live catalog — add (net-new) / enhance (extend an
  existing one) / included (already covered) / superseded (ours is better) —
  every verdict naming what it compared against.
- **INCREMENT (the bridge lens).** What is the next small, valuable slice? The
  backlog is sorted by WSJF (business value ÷ effort); you pull from the top to
  capacity. The pull is the event that ELABORATES an item (raises its
  articulation tier) — not a calendar, not a blanket "analyze everything" pass.
  `plan-increment` runs the pull + the just-in-time elaboration trigger.
- **QUALITY (the derived lens).** What is proven, what is broken, what is
  untested? Never asserted — computed. An item's done-ness is its criteria
  converging; a feature's done-ness is the rollup of its use-cases + deliverables
  (a goal is never met: its distance is read through the features serving it).
  `verify-and-gate` runs the probes through `object-verify` (which records the
  verdicts); `object-verify` green + `object-lint` clean decides.

## Principle 1 — Just-in-time elaboration (defer the detail)

Heavy "plan everything upfront" fails two ways: premature elaboration churns
(delivering one item invalidates the analysis of ten others), and order baked
into detail makes re-sequencing a rewrite. The fix is **less plan, later**:

- A backlog item carries ONLY a title, a one-line value, a `business_value`,
  and an `effort` — until it is pulled.
- On pull, THEN it gets its articulation (its acceptance criteria), its
  placement, its tasks. Elaboration is work IN the increment, not a gate to
  filing the item.
- A method that refuses to create a backlog item without full acceptance
  criteria violates this. (Full doctrine: modules/method-isa/lean-increment-flow.md.)

## Principle 2 — Computed quality (never hand-flipped)

Progress is convergence, not a checkbox:

- An acceptance criterion's result is the `verdict` `object-verify` records beside
  it from the check that ran, not a hand-set status. Only `pass` passes; a check
  that could not run is unproven, never a pass.
- A deliverable reaches `done` only through the verified path: provenance
  (`produced_by` / `produced_at` / `evidence`) + an addressed, passing
  use-case. The gate — `object-verify` green over it and `object-lint` reporting no
  error finding — is what a `done` that skipped it fails.
- A feature's "done" / "verified" is the rollup of its use-cases +
  deliverables — set it on faith and the gate rejects it.

This is why the method is trustworthy without discipline alone: the tool keeps
you honest.

## Worked example — "create a skill", end to end

> The user: "I want a skill that helps write good git commits for this repo."

1. **WANT** — load `isa` (or, for a single skill, the compose
   `skill-intent-interview`). Articulate done: Feature = "an LLM in this repo can
   write a conventional commit (subject rules, body, task-ref)". Criteria
   (each a probe): `the skill's description states WHEN to load it`
   (`resolves:` + a read check); `the body names the subject-line rule`;
   `anti: it does NOT duplicate an existing commit skill`.
2. **HAVE** — load `value-fit`. Scan the catalog: is there a
   `commit-message-style` skill already? If yes and it covers these criteria →
   **included**, stop (record what covers it). If it covers most but lacks the
   task-ref trailer → **enhance** that skill. If nothing → **add**.
3. **INCREMENT** — one small artifact; no backlog needed. (At scale,
   `plan-increment` would pull this feature by WSJF among others.)
4. **DO** — produce the skill (the `documenter` persona / the compose
   `create-artifact` chain). It lands at its `home`.
5. **VERIFY** — load `verify-and-gate`. `object-verify` runs each criterion's probe:
   `exists:app/.../commit-message-style.md` → pass; `resolves:<provider>:/skill/<name>`
   → pass; the anti-criterion (no duplicate) → a catalog check → pass. It records
   one `verdict` per criterion — nothing failed, nothing unproven.
6. **GATE** — the deliverable gets `status: done` + provenance only because its
   addressed use-case passes. `object-verify` green + `object-lint` with no error
   finding → the feature is done, computed.

Same six steps for a task, a reference, a whole sidecar — only the DO chain +
the probe-runner change.

## See also

- `ktw-methods:/skill/create-method` — the map this expands.
- modules/method-isa/lean-increment-flow.md — the increment doctrine (§7 = the
  conformance checklist every method tool must satisfy).
- modules/code/concepts.md — the rock-solid code method: the most mature
  instance of this loop (the spine generalized here).
