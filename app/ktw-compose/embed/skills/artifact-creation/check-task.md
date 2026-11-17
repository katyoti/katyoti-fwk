---
name: check-task
description: "Static checklist for one TASK file; load to validate a task's frontmatter, engine mapping, references, and hygiene before shipping or when auditing for drift."
advertised: false
hashtags: [check, task, lint, audit, hygiene]
category: development
---

# Task check

A static checklist for ONE task artifact. It runs without executing anything —
read the file, resolve what it points at, report pass/fail per item with a
`file:line` for every finding. Works the same on a task you just authored and on
one that shipped long ago (audit mode): the checklist is the contract, the
artifact either meets it or it doesn't.

This is the cheapest gate. It does NOT run the task (that is
`ktw-compose:/skill/test-artifact`) and does NOT score its behavior. It checks
the task is *well-formed, wired, and clean* — true or false by reading. The
contract it checks against is `ktw-compose:/skill/task-format`.

## The checklist

### 1 — Frontmatter contract
- [ ] Only keys from the closed set are present (per `task-format`): `name`,
      `title`, `type`, `description`, `icon`/`iconName`, `hashtags`, `category`,
      `tags`, `capability`, `config`, `permissionProfile`, `group`,
      `sessionMode`, `advertised`, and the CWM targeting keys (`kind: object | collection`,
      `types:`, `filter:`). Flag any stray key — `mode` at root included (it is
      `config.mode`).
- [ ] `name:` is kebab-case and **equals the file stem** (a folder task's entry
      file is `TASK.md` and the folder name is the slug).
- [ ] `type:` is exactly one of `agent` | `task` | `workflow` | `simulator`.
- [ ] `description:` is present and states what the task does — not a restated
      title.
- [ ] `advertised: false` iff the task is an internal worker (delegated by a
      coordinator, never launched directly). A user/action-launched task is
      advertised.

### 2 — Config, not flat frontmatter
- [ ] Strategy params live UNDER a `config:` block, never at root level. Flat
      legacy frontmatter (config fields hoisted to the top) is a fail.
- [ ] `config.mode` is set when siblings in the same family set it; the value fits the
      role (`worker` for delegated work, `chat` for a direct interactive loop,
      `background` for fire-and-forget coordination).
- [ ] The contracts are files beside the task (`<name>.input.dsl.yaml`,
      `<name>.output.dsl.yaml`), never `input-schema:` / `output-schema:`
      frontmatter (the old form the lint names). When present they are ktw-schema DSL
      (`name`, `root`, `types` with `fields:`), never JSON Schema — JSON Schema
      is only ever derived from the DSL.

### 3 — Body maps to the engine
- [ ] An `agent` task's body is a real prompt and orders the **complete** tool
      by name (the run has a defined finish, not an open-ended "report to the
      user") — except a `config.mode: chat` task, which never orders it.
- [ ] The body reaches tools by their canonical name, not by shelling out —
      a task that drives its work through `bash` while its siblings call the
      sidecar's tools by name is an inconsistent-surface fail (unless the task's
      whole purpose is shell orchestration).
- [ ] Every tool the body names in bold is a real tool for the task's mode,
      spelt as registered (kebab-case: `ask-user`, never `ask_user`);
      every skill URI it loads resolves. Dangling `…:tasks/<name>` or
      `…/TASK.md` references (flat tasks are `<name>.md`, not `<name>/TASK.md`)
      are fails.

### 4 — Hygiene: no legacy, no tracking cruft
- [ ] **No task-tracking anywhere in the body or config** — no `US-N`,
      `Track B*`, `Phase X` (as a milestone, not a skill-procedure step name),
      `ms-NNNN`, `sdd-N`, `OD-N`, ticket ids, or `added YYYY-MM-DD`. Tracking
      lives in commits and the module's requests, never in a shipped artifact.
- [ ] **No history/provenance prose with no functional effect** — "will land in
      the future X task", "baked in by track Y", "kept for the old flow".
- [ ] No `TODO`/`FIXME`, no dead cross-refs to deleted artifacts, no
      commented-out alternate instructions. (A body instructing the agent to
      *write* a TODO for the user is functional guidance, not cruft.)
- [ ] No link to a task tracker's path or to files outside this sidecar.

### 5 — Companions
- [ ] If the task is a folder (`<slug>/TASK.md`), its companions (`system.md`,
      fixtures, child task files) exist and are referenced. A workflow task's
      child steps resolve.

## Output

Report the checklist with each item ✅/❌ and, for every ❌, the `file:line` and
the one-line fix. End with a verdict: **clean** (ship/keep), or **N findings**
(fix these first). The items are binary by construction — never soften a ❌ to a
maybe.
