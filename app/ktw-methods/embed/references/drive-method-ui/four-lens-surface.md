# The four-lens surface — concrete view specs

The reference behind `ktw-methods:/skill/drive-method-ui`. The skill is the
map; this is the exact declarations. It does NOT restate the author-ui-* skills
(load those for the extension grammar) — it gives the column sets + bindings a
create-method UI should declare.

## WANT — the Use-Cases table (`ui-table` on `use-case`)

The acceptance surface: every use-case with its rolled-up verdict, ACs
expandable.

```yaml
extensions:
  - type: ui-table
    name: default
    columns:
      - { id: id,           label: "UC",       sortable: true, width: 90 }
      - { id: title,        label: "Use case", sortable: true }
      - { id: severity,     label: "Severity", align: center, width: 110 }
      - { id: status_cache, label: "Status",   align: center, width: 110 }
    pagination: true
    pageSize: 20
    highlightOnHover: true
```

- `status_cache` is the COMPUTED verdict (worst-of its ACs, written by
  verify-and-gate) — badge it `color_by_value` (pass→green, fail/regressed→red,
  no-test→grey).
- The inline `acceptance_criteria` render as an expandable sub-table (claim ·
  kind · test_ref · status) — the per-AC proof.

## HAVE — the Deliverables table (`ui-table` on `deliverable`)

What was produced, where, and the PROVENANCE — the answer to "what was done,
when, by whom?".

```yaml
extensions:
  - type: ui-table
    name: default
    columns:
      - { id: artifact_kind, label: "Kind",   align: center, width: 110 }
      - { id: name,          label: "Name",   sortable: true }
      - { id: home,          label: "Home",   sortable: true }
      - { id: status,        label: "Status", align: center, width: 110 }
      - { id: produced_by,   label: "By",     width: 140 }
    pagination: true
    pageSize: 20
    highlightOnHover: true
```

- `status` badge `color_by_value` (done→green, in-progress→amber, planned→grey,
  dropped→strikethrough). `done` is verify-gated — `produced_by` is non-empty
  exactly when the gate let it through.

## INCREMENT — the cost-of-delay backlog board (`ui-table` on `feature`)

The next-slice surface. A `feature` carries its cost of delay in its `feature`
capability (`cod`); effort is derived (its unfinished use-cases), never stored.
Sort by `cod` descending — "what's next" is a sorted view, not a re-plan.

```yaml
columns:
  - { id: title, label: "Feature",       sortable: true }
  - { id: cod,   label: "Cost of delay", align: center, width: 120, sortable: true }
```

## QUALITY — rollup badges + the action bar

- **Goal-met badge** — bound to the computed rollup (every use-case passes +
  every deliverable done). Green only when `object-verify` is green on the node
  (nothing failed, nothing unproven) and `object-lint` reports no error finding;
  never a hand-set field. This is the node's
  `intention_status` / `status` — but rendered as the COMPUTED verdict, not the
  declared one (the gate refuses a false `verified`).
- **Coverage** — passing ACs / (total − deferred − skipped), shown as a bar.
- **The action bar** — the loop's drive buttons on the node type's `actions:`:
  - an interactive wizard (compose-artifact / asset-intake) → **`kind: ai`**
    (opens a chat tab; the GUI's NodeTab routes `kind: ai` actions to a chat
    launch — see the gui-binding work);
  - a headless run (verify, project) → default (`sync`/`async`, fire-and-forget).
  - Every action carries `label` + `description` (a bare URI renders as the raw
    URI). Per `ktw-compose:/skill/create-object-type`.

## Composing them — the node page

The three tables + the goal `ui-view` + the badges compose into the node's page
(a `page.yaml` on the node type, or the schema-default CP-3a layout refined by
the per-data-type extensions above). The TABLES + badge bindings ship now (on the
data-types) — they are the durable, type-driven part you declare today.

The PAGE that arranges them into a single node screen — and the live render of
the per-item tables on a node's detail — depend on the **per-node-type
page-layout mechanism** (CP-3c in `dimension-ui.md`) and item-table rendering on
node detail. The GUI does not render those yet (node detail is still a fixed
layout). So: declare the lenses now; they compose into the node page once that
render path lands. Until then, preview any single lens live with
`ktw-methods:/skill/compose-ui-live`.

## See also

- `ktw-methods:/skill/drive-method-ui` — the map this expands.
- `ktw-methods:/skill/author-ui-table` · `author-ui-view` · `author-ui-form` —
  the extension grammar.
- modules/display/ — the one display definition (`render.Def`) every surface renders from.
