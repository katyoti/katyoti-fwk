---
name: check-reference
description: "Static checklist for one reference doc; load to confirm it is passive material (not a mis-shaped skill), keeps its URI extension, is actually read, and has resolving in-sidecar links."
advertised: false
hashtags: [check, reference, lint, audit, hygiene]
category: development
---

# Reference check

A static checklist for ONE reference — a passive lookup doc under
`embed/references/` (a schema, a table, a catalog, a convention sheet). It runs
without executing anything: read the file, resolve what it points at, report
pass/fail per item with a `file:line`. A reference has no frontmatter contract —
it is raw material — so the checks are about *what it is* and *whether anything
reaches it*.

## The checklist

### 1 — It is really a reference (not a mis-shaped skill)
- [ ] The content is **facts to read** — a schema, a table, a catalog, a
      convention — with **no procedure and no decision points**. If it tells an
      LLM *how to proceed through steps with choices*, it is a **skill**: flag it
      and route to `ktw-compose:/skill/create-skill`.
- [ ] It is not better expressed as a **tool** that returns the value (a live
      config value, a credential-derived base) — a reference is for static
      material only.

### 2 — Identity and reach
- [ ] Its URI is `…:/reference/<filename>` and **keeps the extension**
      (`rich-markdown.md`, not `rich-markdown`). Reference ids include the suffix.
- [ ] **Something reads it.** At least one skill/task in the sidecar names this
      reference by URI. A reference no artifact reaches is dead weight — flag it
      (delete, or wire the consumer that was supposed to use it).

### 3 — Links resolve and stay in-sidecar
- [ ] Every URI / relative link in the body resolves to a real artifact.
- [ ] **No link leaves the sidecar** — no path into another sidecar's source, no
      `../../..` escape, no link into a task tracker's folder. A deployed sidecar cannot reach
      those; the link is dead on arrival.

### 4 — Ships and is wired
- [ ] `references/` is covered by `//go:embed` (the sidecar's embedded pack
      publishes it). Not embedded = a reference that resolves to nothing.

### 5 — Hygiene
- [ ] No `US-N`, `Track B*`, `ms-NNNN`, ticket ids, or `YYYY-MM-DD`-as-provenance.
      No `TODO`/`FIXME`, no "renamed from / previously" history prose. Illustrative
      dates inside sample data are fine; provenance dates are not.

## Output

Report each item ✅/❌ with a `file:line` and one-line fix for every ❌. The
highest-value findings are §1 (this should have been a skill/tool) and §2 (nobody
reads it) — both mean the artifact is in the wrong shape or is dead weight. End
with **clean** or **N findings**. Items are binary — never soften a ❌.
