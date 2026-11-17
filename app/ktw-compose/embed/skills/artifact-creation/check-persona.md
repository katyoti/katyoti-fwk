---
name: check-persona
description: "Static checklist for one persona; load to validate its frontmatter, augments baseline, resolvable skills/plugins, ship-wiring, and hygiene when authoring or auditing."
advertised: false
hashtags: [check, cwm, persona, agent, lint, audit]
category: development
---

# Persona check

A static checklist for ONE persona (`personas/<name>/TASK.md`). It runs without
executing anything: read the file, resolve what it points at, confirm it ships,
report pass/fail per item with a `file:line`. The contract it checks against is
`ktw-compose:/skill/create-persona` (and, for the agent-loop half,
`ktw-compose:/skill/create-agent`). It does NOT run the persona (that is
`ktw-compose:/skill/test-artifact`).

## The checklist

### 1 — Frontmatter contract
- [ ] `name:` is kebab-case and equals the persona **folder** name; YAML parses.
- [ ] `type: agent-loop` and `category: persona` (else it is not a library
      persona — it is a plain task, check it as one).
- [ ] `description:` present, one line, states the role and its job.
- [ ] The binding fields are present: `bind_mode` (`object` | `global` | `custom`), `types`, and
      `skills` + `config.plugins` (the agent surface). The retired
      `allowed_node_types` is a fail (the persona is refused).

### 2 — It actually ships
- [ ] `personas/<name>/TASK.md` lives under a folder the sidecar embeds + wires
      (the persona-library loader). Embedded-but-unwired = a persona that never
      reaches the library — flag with the missing side.

### 3 — The layered library
- [ ] If `augments:` is set, a **baseline persona of that name exists** to layer
      onto; the merge (unioned skills/plugins, appended body) is the intent.
- [ ] No two same-named **baselines** (a persona with no `augments`) exist across
      the library — the merge would be ambiguous. Flag a duplicate baseline.
- [ ] An augment specific to some work narrows itself with `applies_to`
      (`files` globs, `modules`); an unnarrowed language- or module-specific
      layer leaks into every spawn of the baseline.

### 4 — References resolve, binding is sound
- [ ] Every skill in `skills:` and every plugin in `config.plugins` resolves to a
      real artifact — a named-but-absent skill sends the bound agent flailing.
- [ ] Every type in `types` exists; `default_gate` (if set) is a real
      DoD criterion the persona actually owns.

### 5 — Body + hygiene
- [ ] The body is framed as a standing ROLE on a bound object (its responsibilities,
      preferred methods/tasks, what not to hand-edit) — not a generic wall; the
      generic object context belongs in its type's or capabilities' `agent:`
      block, not here.
- [ ] Every tool the body names is spelt as registered (kebab-case:
      `file-read`, `ask-user`), in **bold**.
- [ ] No `US-N`, `Track B*`, `ms-NNNN`, ticket ids, or dates-as-provenance; no
      `TODO`/`FIXME`; no links into a task tracker's folder.

## Output

Report each item ✅/❌ with a `file:line` and one-line fix for every ❌. Call a
duplicate baseline or a dead (unshipped) persona out loudly. End with **clean** or
**N findings**. Items are binary — never soften a ❌.
