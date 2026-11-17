---
name: check-object-type
description: "Static checklist for one CWM object-type YAML; load to validate its shape, capabilities, containment, action targets, schema binding, ship-wiring and hygiene before shipping or when auditing."
advertised: false
hashtags: [check, cwm, object-type, lint, audit]
category: development
---

# Object-type check

A static checklist for ONE CWM object type — an `embed/cwm/object-types/*.yaml`.
It runs without executing anything: read the file, resolve what it points at,
confirm it actually ships, report pass/fail per item with a `file:line`. It works
the same on a type you just wrote and on one that shipped long ago (audit mode).

The contract it checks against is `ktw-compose:/skill/create-object-type`. It does
NOT test the GUI panels the type drives (that is `ktw-compose:/skill/test-gui`).

## The checklist

### 1 — Shape
- [ ] `name:` is kebab-case and equals the file stem.
- [ ] `label` and `description` are present; the description says how the type
      differs from its nearest sibling.
- [ ] Only object-type fields, plus sections: every other top-level key is the
      name of a capability (its settings on the type, checked against its
      `type_schema`). A key naming neither refuses the type. Retired keys are a
      fail: `strategy`, `extends`, `allowed_item_types`, `type_specific_data_type`,
      `schema_data_type`, `storage`, `injected_items`; and at top level `view`
      (now `pane.view`), `gate_blocks_on` (now `verification.gate_blocks_on`),
      `propagation` / `resolution_strategy` / `override_key` /
      `briefing_fields` / `injected_objects` (now under `context:`).
- [ ] `tree:` is `root`, `node`, `none` or absent; a `folder:` does not start
      with `_`.
- [ ] YAML parses; no duplicate keys.

### 2 — It actually ships (the ships-invisible trap)
- [ ] The `embed/cwm` folder is covered by a `//go:embed` line in the sidecar's
      `embed.go`.
- [ ] The sidecar's `bootstrap.go` keeps the kit's default pack (no
      `sidecarkit.WithoutPack()`): the pack reads `embed/cwm` as a part of it,
      with no option to pass. A file outside the embedded folder, or a pack
      dropped, is a **dead artifact** that reaches no catalog: flag it with the
      missing side and recommend embed-or-delete.

### 3 — Capabilities and fields
- [ ] Every name in `capabilities:` is a capability declared in this pack or one it
      depends on (`embed/cwm/capabilities/<name>.yaml`).
- [ ] `fields:` resolves to a real `…:/data-type/<id>`; the DSL file stem equals
      that id.
- [ ] No field is duplicated between the type's `fields` and a capability it
      lists — a shared or behaviour-bearing group belongs to the capability.
- [ ] A lifecycle state is not a plain field: the type names a `workflow:` and the
      state lives in the `lifecycle` capability.
- [ ] Each section's capability exists and the section matches its
      `type_schema` (the pack's own section — `signoff: {required_reviewers: 2}`
      — as much as `pane:`, `context:`, `verification:`).

### 4 — Containment
- [ ] The type is creatable somewhere: either it sets `accept_parents`, or some
      type lists it in `allowed_children`. A drawn top-level type (`tree: root`)
      sets `accept_parents: ['*']`.
- [ ] Names in `accept_parents` / `allowed_children` are types that exist (own pack
      or a declared dependency).

### 5 — Actions resolve, and intent is distinguished from breakage
- [ ] Every `actions[].uri`, `create_task`, hook entry (`before_*`, `on_*`,
      `lint`, `verify`) is a valid action URI and its target exists. A dangling
      target is a fail.
- [ ] A refusing hook (`before_*`, `on_transition`) on a type this provider does
      not own does not apply without a Univers.yaml grant
      (`sidecars[].grants.gate`): `ktw-compose lint-universe` lists it.
- [ ] An action marked `preview: true` whose target does not exist yet is a
      **deliberate placeholder** (the GUI shows "À venir"), NOT a broken link —
      report it as *planned*.
- [ ] Each action has a `label`, a `description` and a deliberate `kind:` (`ai` for
      an interactive wizard → chat tab; omitted for a headless run).
- [ ] Every `when:` reads the object vocabulary (`object.<capability>.<field>`,
      `object.fields.<key>`, `id`, `type`, `parent_id`, `children`). A `node.*` /
      `item.*` name does not compile: the action shows disabled with the error.

### 6 — Inheritance and translations
- [ ] In the `context:` section, `resolution_strategy: closest-wins` or
      `accumulate-with-local-override` comes with an `override_key` that exists
      in the fields — without it the resolution is silently useless.
- [ ] A `<name>.i18n.yaml` beside the type gives `label` (and `description`,
      action and badge labels) in every supported language other than the
      source: `ktw-compose i18n <pack> --lang fr --missing` answers `complete`.

### 7 — Hygiene: no tracking cruft
- [ ] No `US-N`, `Track B*`, `ms-NNNN`, `sdd-N`, `OD-N`, ticket ids, or
      `YYYY-MM-DD`-as-provenance in comments or field values (`agent.system` text and
      `description` are user/agent-facing prose — same rule as a skill body).
- [ ] No history/provenance comments with no functional effect; no `TODO`/`FIXME`;
      no links into a task tracker's folder.
- [ ] `agent.system` text / `description` say what the object IS and how work flows
      through it now — not how it got here.

## Output

Report the checklist with each item ✅/❌ and, for every ❌, the `file:line` and
the one-line fix. Call out a dead (unshipped / unwired) type loudly — it is the
highest-severity finding because nothing surfaces it. End with a verdict:
**clean** (ship / keep), or **N findings**. Items are binary — never soften a ❌.
