---
name: deliverable-regenerate
title: "Régénérer le livrable"
description: "Réécrit un livrable (un document portant la capacité deliverable) depuis son modèle et les objets dont il est issu : mêmes fichiers, même public, même langue."
type: task
iconName: refresh
icon: "🔄"
hashtags: [docs, deliverable, document, regenerate]
category: documents
config:
  mode: worker
  capability: smart
  plugins:
    - cwm             # cwm-object-get / cwm-edge-list / cwm-object-file-* / cwm-object-update
    - object-context  # the deliverable the action was run on
    - task            # complete
  tools:
    - ktw-docs:docs/render
    - ktw-docs:docs/render-template
---

# Régénérer le livrable

You write one deliverable again from what it was written from. The deliverable
is the current CWM object: its id is the `object="…"` attribute of the
`<cwm_object_context>` block.

## Step 1 — Read the deliverable

Call `cwm-object-get` on the id. Note its `deliverable` group: `template` (what
wrote it), `audience`, `language`, `format`, `state`. Its body, when it has
one, is its prose. Call `cwm-object-file-list` for the files attached to it:
its main file is the one whose extension matches `format`.

## Step 2 — Read what it was written from

Call `cwm-edge-list` on the id and keep the `produced-from` edges. Read each
target with `cwm-object-get`, and the files attached to it that the
deliverable cites. Facts come from these objects only, never invented.

## Step 3 — Write it again

Follow the template: a skill named `doc-<template>` when one is offered to
you, else the content template of that name. Keep the audience and the
language. Rewrite the attached main file with `cwm-object-file-write` (same
path), or the body with `cwm-object-update` when the prose is the body
(`cwm-object-get` it first and pass the `revision` it gave). A
`word` or `pdf` format: write the Markdown first, then `docs/render` it beside
it.

## Step 4 — Complete

Call `complete` with the files rewritten and the objects read. When the
deliverable was `validated` or `published`, say it changed and must be
reviewed again: a person moves it back through its workflow.
