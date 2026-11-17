---
name: apply-delta
type: agent
description: "Apply a demand's approved delta through the verbs, operation by operation, and answer the object each one changed or created"
iconName: checks
icon: "✅"
hashtags: [planification, demand, delta, apply]
category: planning
config:
  mode: worker
  capability: smart
  maxTurns: 40
  plugins:
    - standard
    - bash   # the ktw-agents verbs
---

# Apply a demand's delta

Read the demand (`ktw-agents requests get --id <demand> --out json`): its `delta` is the
approved list. Apply each operation through its verb, never by editing a file:

| op | verb |
|---|---|
| `goal-add` | `ktw-agents action run --uri ktw-methods:goals/add --input '{"module":…,"title":…}'` |
| `feature-add` | `ktw-agents action run --uri ktw-methods:features/add --input '{"module":…,"title":…,"serves":["<goal>"]}'` - `under` a goal goes in `serves`, `under` a feature in `"parent"`; then the op's `details` become the feature's body: `ktw-agents cwm object-get` it, set its body, `ktw-agents cwm object-update` it with the revision read |
| `goal-change` / `feature-change` | `ktw-agents cwm object-update` |
| `increment-create` | `ktw-agents increments create --module <module> --stream <stream> --title <title>` - the op's stream: an increment is always created in its stream |
| `increment-done` | none: closing an increment is a person's (`increments done` refuses an agent) - its delivery approval reaches « À faire » once everything it holds is done; report it applied with the increment's id |
| `increment-target` | `ktw-agents increments target --id <object> --increment <increment> --demand <this demand>` - naming the demand aims it at once (without it, a running increment takes the goal or feature only as a new proposal) |
| `request-rescope` / `merge` / `supersede` | `ktw-agents requests rescope` / `merge` / `supersede` |
| `request-drop` | `ktw-agents plans finish --outcome dropped` |
| `direct` | `ktw-agents requests file --kind fix` then `requests assess --depth quick` |

An operation the request's `applied` already names was applied in an earlier round (an
increment's goals changed after its proposal): skip it. Apply them in order: a field `#N` (`under`, `object`, `increment`) names what operation N
made — use the id you got for it.

Report under `more.applied` one entry per operation: `{"op": <its index>, "id": "<the
object it changed or created>"}`. Planification checks that every operation is covered,
then links the demand to each object (`realized-by`) and tells its requester. A goal or
feature operation that names an `increment` is aimed at it by planification once your
report is collected: do not target it yourself.
