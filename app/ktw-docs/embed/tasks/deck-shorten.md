---
name: deck-shorten
title: "Version courte de la présentation"
description: "Fait une version courte d'une présentation (un objet slide-deck) pour un temps de parole donné : une copie à côté de l'originale, l'histoire gardée, les détails en annexe ou retirés — mêmes thème et mises en page."
type: task
iconName: scissors
icon: "✂️"
hashtags: [slidedeck, deck, shorten, summary]
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

# Version courte de la présentation

You make a short version of one slide deck for `minutes` of speaking time
(from the `FORM_INPUT` block; default 10) and store it as a NEW deck beside
the original. The original is never changed.

The deck is the current CWM object: its id is the `object="…"` attribute of
the `<cwm_object_context>` block. Its module is the id's prefix.

## Step 1 — Read the deck

Call `cwm-object-get` on the id: its `name`, its `deck` capability and its
`body` (the slides, in the deck-author grammar). A body that starts with a
`---` frontmatter block carries settings that belong in the `deck` fields:
leave that block out of the copy's body.

## Step 2 — Keep the story

The short version has AT MOST `minutes` slides, the cover and the closing
slide included — a hard limit, not an aim. A deck already within it still
loses a third of its slides: a short version that is not shorter is a
failure. Keep the cover, the
message of each part, the key figures and the decision or the ask, and the
closing slide. Merge slides that say the same thing; cut detail, examples and
backup material. Never change a figure, a name or a fact, and never write a
sentence the original does not support. Keep layouts, variants, icons, file
paths, diagrams and the speaker notes of the slides you keep.

When `appendix` is true, put the slides you removed, unchanged,
as vertical slides (`--`) under a last `section` slide titled « Annexes » (in
the deck's language) instead of dropping them.

## Step 3 — Store the copy

Before storing, count the horizontal slides (those separated by `---`;
the appendix's `--` slides do not count) and cut again until the count is
within the limit.

Call `cwm-object-create` in the same module: type `slide-deck`, the name
followed by « — version courte » (in the deck's language), the same `deck`
fields, the new `body`, under the same parent as the original. Call
`object-lint` on it and fix every finding (`cwm-object-get` it, then
`cwm-object-update` with the `revision` it gave) until there is none.
Unsure how a slide renders: deck-author, « If you are unsure how a deck
renders » (optional).

## Step 4 — Complete

Call `complete` with the new deck's id and name, the number of slides of each
version, and what was moved to the appendix or removed.
