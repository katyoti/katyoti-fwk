---
name: deck-translate
title: "Traduire la présentation"
description: "Traduit une présentation (un objet slide-deck) dans une autre langue : une copie à côté de l'originale, mêmes mises en page, même thème, mêmes schémas — seuls les textes et les notes de l'orateur changent."
type: task
iconName: language
icon: "🌐"
hashtags: [slidedeck, deck, translate, language]
category: documents
config:
  mode: worker
  capability: smart
  plugins:
    - cwm             # cwm-object-get / cwm-object-create
    - object-context  # the deck the action was run on
    - verification    # object-lint
    - task            # complete
  skills:
    - deck-author
---

# Traduire la présentation

You translate one slide deck into `language` (from the `FORM_INPUT` block;
default `en`) and store the translation as a NEW deck beside the original.
The original is never changed.

The deck is the current CWM object: its id is the `object="…"` attribute of
the `<cwm_object_context>` block. Its module is the id's prefix.

## Step 1 — Read the deck

Call `cwm-object-get` on the id. Note its `name` (the deck's title), its
`deck` capability (aspect, theme, transition, author, watermark, mark) and its
`body` (the slides, in the deck-author grammar). A body that starts with a
`---` frontmatter block carries settings that belong in the `deck` fields:
leave that block out of the copy's body.

## Step 2 — Translate

Translate, slide by slide, everything a person reads: headings, text, list
items, table cells, image labels (`![label](…)`), quotes and their author, and
the speaker notes after `Note:`. Keep EXACTLY, byte for byte:
- every structural line: `---`, `--`, `<!-- slide: … -->`, `::slot::`;
- layout, variant and setting values; `icon:<name>` and every file path;
- code blocks and ```` ```mermaid ```` blocks (translate only the quoted
  labels inside a mermaid diagram, nothing else);
- numbers, names of products, systems and people, acronyms.
Keep the same number of slides and the same order. Translate `name` too. Use
the register of the original (formal stays formal).

## Step 3 — Store the copy

Call `cwm-object-create` in the same module: type `slide-deck`, the
translated `name`, the same `deck` fields, the translated `body`, under the
same parent as the original. Then call `object-lint` on the new deck and fix
every finding (`cwm-object-get` it, rewrite with `cwm-object-update` passing the
`revision` it gave) until there is none.

## Step 4 — Complete

Call `complete` with the new deck's id and name, the language, and the number
of slides; say the original is unchanged.
