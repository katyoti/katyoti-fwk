---
name: drive-method-ui
description: >-
  Make a creation method DRIVABLE from the GUI — declare the four-lens
  surface (want / have / increment / quality) on the method's CWM types so a
  user sees what they want, what they have, the next slice, and the computed
  quality, and drives it with action buttons. Load when wiring the UI for a
  composition / module / dossier (or any create-method node), or to decide
  which type-driven-ui dimension renders which lens. It routes to the
  author-ui-* skills; it does not re-implement them.
iconName: layout-dashboard
icon: "🖥️"
hashtags: [ui, drive, dashboard, type-driven, methodology, lens]
category: methodology
---

# Drive-method-ui — make the method drivable

`ktw-methods:/skill/create-method` is the loop; THIS skill is how a user
DRIVES it from the GUI. You do not hand-build a React screen — you **declare**
the four-lens surface on the method's CWM types, and the type-driven UI renders
+ drives it. This skill is the map of WHICH dimension renders WHICH lens and
WHICH `author-ui-*` skill authors it.

## The four-lens surface

A create-method node (a `composition`, a code `module`, a da dossier) renders
the loop's four lenses (see `create-method`). Each lens is one declarable
surface:

| Lens | The surface | Declared as | Author with |
|---|---|---|---|
| **WANT** | Goal prose + the **Use-Cases table** (id · title · severity · status, expandable ACs) | a `ui-view` on the node body + a `ui-table` on the `use-case` data-type | `ktw-methods:/skill/author-ui-view` · `author-ui-table` |
| **HAVE** | the **Deliverables table** (kind · name · home · status · **produced_by**) | a `ui-table` on the `deliverable` data-type | `ktw-methods:/skill/author-ui-table` |
| **INCREMENT** | the **cost-of-delay backlog board** (feature · cod · pull state) | a `ui-table` on the `feature` objects (the `cap-feature` fields) | `ktw-methods:/skill/author-ui-table` |
| **QUALITY** | computed **rollup badges** (goal-met · coverage · gate-pass) + the **action bar** (verify / gate; `kind: ai` for wizards) | state badges via `color_by_value` schema annotations + the node-type `actions:` | `ktw-methods:/skill/author-ui-view` + `ktw-compose:/skill/create-node-type` |

Live, in-chat preview of any of these while you build:
`ktw-methods:/skill/compose-ui-live`. A produced-data dashboard (KPIs / charts
from a sidecar's output): `ktw-methods:/skill/render-data-as-ui`.

## The three rules

1. **Declare, don't draw.** Put the table columns / view panels / badges in the
   data-type's `extensions:` block (a `ui-table` / `ui-view` per the author-ui-*
   skills) — the GUI renders from the declaration. Never hand-author a screen
   per type.
2. **Render the COMPUTED state, not a status field.** The QUALITY lens shows the
   rollup the gate computes (goal-met, coverage, every-AC-pass) — a badge bound
   to the computed value, never a hand-set flag. (The values come from
   `verify-and-gate` — `object-verify`'s verdicts + `object-lint`'s findings.)
3. **Actions are the drive surface.** The action bar launches the loop's
   stages — an interactive wizard (the WANT/DO step) is `kind: ai` (opens a chat
   tab); a headless run is the default. Declared on the node type's `actions:`
   (see `ktw-compose:/skill/create-object-type`).

## Reference + first instance

- `ktw-methods:/reference/drive-method-ui/four-lens-surface.md` — the concrete
  specs: the exact Use-Cases + Deliverables table columns (incl. provenance), the
  rollup-badge bindings, the action-bar `kind: ai` rule.
- The **composition display view** is the first instance: the
  `ui-table` extensions on the `use-case` (WANT) and `deliverable` (HAVE)
  data-types ship in the methods pack; the page that composes them + the rollup
  badges land with the GUI binding work.

A new method gets a drivable UI for free by declaring these four surfaces on its
own types — no bespoke screen.
