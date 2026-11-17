---
name: overlay-iteration
description: "Author and iterate an embedded sidecar artifact without recompiling — write to the workspace overlay, test live, promote when validated — for sidecars whose source is in this workspace."
advertised: false
hashtags: [authoring, overlay, overwrite, iterate, promote, create-items]
category: guide
---

# Iterate an embedded artifact without recompiling

An embedded artifact normally ships inside its sidecar binary, so every edit
needs a rebuild. When the sidecar's source lives in THIS workspace's repo, skip
that loop: write to the **workspace overlay** instead. The sidecar serves an
overlay file as if it were embedded — live, no `go build`. You promote into the
source tree once, at the end.

Use this whenever you author or refine an embedded skill / task / persona /
reference and the rebuild loop would otherwise dominate the iteration.

## When it applies

- The target sidecar's source is in this repo (`app/<sidecar>/` exists), AND
- the universe opts in: `allow_overwrites: true` in its `Univers.yaml`.

If the universe gate is off, the overlay file is written but IGNORED — enable
the gate (dev universes only) or the artifact won't be served. The create tool
warns you when the gate is off.

If the sidecar is NOT in this workspace, this path doesn't apply — author into
`<workspace>/.katyoti/skills/` (host-loaded, no rebuild) or accept the rebuild
for a true embedded change.

## 1. Author into the overlay

Call the create tool with `target: "overwrite"` — it writes to
`.katyoti/overwrites/<sidecar>/embed/<type>/<name>` and the sidecar serves it
live:

- **Call** `ktw-compose:compose/create-skill` with
  `{"sidecar": "<name>", "name": "<skill>", "description": "<L0 line>", "advertised": false, "target": "overwrite"}`.
- Same `target: "overwrite"` on **`ktw-compose:compose/create-task`**,
  **`ktw-compose:compose/create-persona`**, **`ktw-compose:compose/create-reference`**.

To edit an existing embedded artifact, copy its body into the overlay file and
adjust there — the overlay shadows the embedded copy. A malformed overlay falls
back to the embedded version, so a broken edit never blanks the artifact.

**CWM types** are overlay-covered too — there is no create verb; author the YAML
by hand into `.katyoti/overwrites/<sidecar>/embed/cwm/{node,item,edge}_types/<name>.yaml`.
It's served live per-sidecar (same gate); an override shadows the embedded type,
a new type is added.

## 2. Test + evaluate live

Iterate freely: edit the overlay file, and the next read/run sees the change —
no rebuild. Test and evaluate exactly as normal (the artifact is served from its
real URI). Repeat until it validates.

## 3. Promote when validated

Once the artifact passes its checks, fold it back into the source tree:

- **Call** `ktw-compose:compose/promote` with
  `{"sidecar": "<name>", "path": "<type>/<name>.md"}` (e.g. `skills/foo.md`).

Promote copies the overlay file into `app/<sidecar>/embed/<type>/` and REMOVES
the overlay copy — the overlay always wins, so it must go once the source is
authoritative. Then rebuild the sidecar and open a PR for the source change, per
the note promote returns.
