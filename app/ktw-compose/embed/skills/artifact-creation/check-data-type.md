---
name: check-data-type
description: "Static checklist for one schema-DSL data-type; load to validate its filename identity, ui-form and reference placement, status-is-meta, wiring, and hygiene when authoring or auditing."
advertised: false
hashtags: [check, data-type, schema, dsl, lint, audit]
category: development
---

# Data-type check

A static checklist for ONE schema-DSL file (`<name>.dsl.yaml`). It runs without
executing anything: read the file, resolve what it points at, report pass/fail
per item with a `file:line`. The contracts it checks against are
`ktw-compose:/skill/create-object-type` and the schema-DSL spec.

Several of these items are silent-failure traps — a wrong placement parses fine
but drops the feature. Check them literally.

## The checklist

### 1 — Identity from the filename
- [ ] The data-type id derives from the **filename stem**, and the inner `name:`
      **equals that stem** (`foo.dsl.yaml` ⇒ `name: foo`). This is the opposite of
      a skill; a stem/name mismatch means the type resolves under a URI nobody
      expects.
- [ ] The file has a single `root:` type; YAML is well-formed; no duplicate keys.

### 2 — Placement traps (parse-clean but feature-dropping)
- [ ] **`extensions:` (ui-form / ui-view / ui-table) is nested UNDER the root
      type, not at the top level.** A top-level `extensions:` block is DROPPED by
      the parser — the form then renders its fields alphabetically instead of in
      the authored order. This is the single most common data-type defect.
- [ ] Any **reference URI keeps its extension** (`…:/reference/foo.md`, not
      `…/foo`). Reference ids are the filename WITH the suffix.
- [ ] Link/relation targets (`->Type`, `~Type`) name a type that exists — no
      orphan refs.

### 3 — A state is a lifecycle's, never a field
- [ ] A type's `fields` data-type or a capability's `schema` declares no
      `status` / `state` field standing for where the object is in its life —
      that is `capabilities.lifecycle.state`, moved by `ObjectTransition` along
      the type's workflow. A field duplicating it drifts from it. (A
      capability with a state of its own declares a `workflow:` on the
      capability, not a free field.)
- [ ] A field holding another object's id is `string@object-ref(types:…)` (or
      `string@panel-ref(…)` for `<id>#<panel>`), so a form draws a picker — not a
      bare string — and a relation that is the object's own meaning is an edge,
      not an id in a field.

### 4 — Content is well-formed
- [ ] Field kinds/content-types are valid and compatible (a content-type applies
      only to the kind that supports it — the string/number/boolean catalog).
- [ ] Enum/select options are well-formed: a bounded multi-select is
      `string[]` + `items.enum`; rich options are `{value, label, description,
      icon}` objects, not bare strings when labels are intended.

### 5 — Generated, versioned, translated
- [ ] A file headed `GENERATED … DO NOT EDIT` is rendered from a Go type: it is
      changed in the Go type and regenerated, never by hand.
- [ ] `version:` is `major.minor.patch`; a change only adds optional fields
      within a major (a new key is a new minor; removing or retyping one is a
      new major).
- [ ] Field `label`s and `description`s a person reads in a form are translated
      in `<name>.dsl.i18n.yaml` beside the file (`fr: {types: {<Type>: {fields:
      {<field>: {label, description}}}}}`).

### 6 — Ships and is wired
- [ ] The `data-types/` folder is covered by `//go:embed` (a sidecar's
      embedded pack publishes it; a binary not built with sidecarkit reads it
      with `ContributePack`). Not embedded = a dead type that reaches no catalog.

### 7 — Hygiene
- [ ] No `US-N`, `Track B*`, `ms-NNNN`, ticket ids, or `YYYY-MM-DD`-as-provenance
      in any `description`/comment. No `TODO`/`FIXME`, no links into a task tracker's folder.

## Output

Report each item ✅/❌ with a `file:line` and one-line fix for every ❌. Call the
placement traps (§2) and a dead/unwired type (§6) out loudly — they are
parse-clean, so nothing else surfaces them. End with **clean** or **N findings**.
Items are binary — never soften a ❌.
