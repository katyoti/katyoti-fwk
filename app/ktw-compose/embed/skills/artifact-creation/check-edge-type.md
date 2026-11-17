---
name: check-edge-type
description: "Static checklist for one CWM edge-type YAML; load to validate its shape, endpoint resolution, storage, ship-wiring, and hygiene when authoring or auditing."
advertised: false
hashtags: [check, cwm, edge-type, lint, audit]
category: development
---

# Edge-type check

A static checklist for ONE CWM edge-type asset (`edge-types/<name>.yaml`). It runs
without executing anything: read the file, resolve what it points at, confirm it
ships, report pass/fail per item with a `file:line`. The contract it checks
against is `ktw-compose:/skill/create-edge-type`.

## The checklist

### 1 — Shape
- [ ] `name:` is kebab-case and equals the file stem; YAML parses; no duplicate keys.
- [ ] Fields present: `from`, `to`, `cardinality`, `inverse_name`, `description`;
      no key outside the edge-type schema (`storage`, `writer`, `freshness` are
      not edge-type keys).
- [ ] `cardinality` is one of `1:1` | `1:N` | `N:1` | `M:N` — anything else is
      refused at load and the edge type is not registered.

### 2 — It actually ships (the ships-invisible trap)
- [ ] The `embed/cwm` folder is covered by a `//go:embed` line in the sidecar's
      `embed.go`: the kit's default pack reads `embed/cwm` with no option (a
      bootstrap passing `sidecarkit.WithoutPack()` drops it). A file outside the
      embedded folder, or a binary built `WithoutPack`, is a **dead edge type**
      that reaches no catalog: flag it with the missing side, and recommend
      embed-or-delete.

### 3 — Endpoints resolve, and extend-before-create held
- [ ] `from` and `to` each name an object TYPE that exists (or `any`); every
      name in `from_capabilities` / `to_capabilities` is a declared capability.
      An endpoint pointing at a non-existent type or capability is a fail.
- [ ] The relationship is not a near-duplicate of an existing edge type
      (`dependencies`/`verifies`/`impacts`/`supersedes`/…). A domain edge that
      restates a methods-vocabulary one should have reused it — flag as a smell.

### 4 — Storage and direction are coherent
- [ ] `object_storage: field` comes with a `field` (`left.parent`, or
      `<side>.<capability>.<property>`); `edges` (the default) needs none.
- [ ] `inverse_name` is present and reads correctly from the `to` side
      (`verifies` → `verified-by`).
- [ ] `label`, `inverse_label` and `description` are translated in a
      `<name>.i18n.yaml` beside the file for every supported language other
      than the source.

### 5 — Hygiene
- [ ] No `US-N`, `Track B*`, `ms-NNNN`, ticket ids, or `YYYY-MM-DD`-as-provenance
      in the `description`/comments. No `TODO`/`FIXME`; no links into a task tracker's folder.

## Output

Report each item ✅/❌ with a `file:line` and one-line fix for every ❌. Call a
dead (unshipped/unwired) edge type out loudly — it is parse-clean, so nothing else
surfaces it. End with **clean** or **N findings**. Items are binary — never soften
a ❌.
