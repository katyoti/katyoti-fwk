---
name: deliverable-export
title: "Exporter le livrable"
description: "Produit une version Word ou PDF d'un livrable (un document portant la capacité deliverable) et la rattache au livrable, à côté de son fichier."
type: task
iconName: file-export
icon: "📤"
hashtags: [docs, deliverable, document, export, word, pdf]
category: documents
config:
  mode: worker
  capability: fast
  plugins:
    - cwm             # cwm-object-get / cwm-object-file-list / cwm-object-file-attach
    - object-context  # the deliverable the action was run on
    - task            # complete
  tools:
    - ktw-docs:docs/render
---

# Exporter le livrable

You export one deliverable to `to` (from the `FORM_INPUT` block; default
`docx`). The deliverable is the current CWM object: its id is the
`object="…"` attribute of the `<cwm_object_context>` block.

## Step 1 — Find its Markdown

Call `cwm-object-get` on the id, then `cwm-object-file-list`: the source is
the attached `.md` file. A deliverable whose prose is its body has no such
file: write the body to `<module folder>/documents/<id>.md` with
`cwm-object-file-write` first.

## Step 2 — Render

Call `docs/render` with the Markdown file as input and `to` as the format; the
output goes beside it, same name, new extension.

## Step 3 — Attach and complete

Call `cwm-object-file-attach` on the id with the output path, so it is listed
in the deliverable's Files tab. Call `complete` with the output path.
