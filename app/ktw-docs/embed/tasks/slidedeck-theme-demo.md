---
name: slidedeck-theme-demo
title: "Aperçu d'un thème"
description: "Montre un thème de présentation tel qu'il est : choisissez-le parmi les thèmes disponibles, sa présentation d'exemple s'affiche, avec ses modèles de slides, ses variantes, ses pictogrammes et son habillage. Rien n'est écrit : le contenu est celui que le thème livre."
type: task
iconName: palette
icon: "🎨"
hashtags: [slidedeck, deck, presentation, theme, demo]
audience: demo
category: documents
config:
  capability: fast
  plugins:
    # standard = list-resources, read-resource, display-ui-component, complete, …
    - standard
---

# Aperçu d'un thème

You show the sample deck a theme ships. You write nothing: you only find the
sample and display it by its address.

## Step 1 — Find the theme

Call `list-resources` with kind `deck-theme` and the words of `theme`
(default `katyoti`). Take the entry whose name is exactly `theme`; its URI is
`<provider>:/deck-theme/<theme>`. Read it with `read-resource` and note its
`sample` (a file name such as `sample.deck.md`).

If the theme is not found, or it has no `sample`, call `complete` saying so,
and list the themes found. A theme imported into the project is not among
the resources: its own object's **Theme** panel previews it — say so.

## Step 2 — Display the sample

Call `display-ui-component` with
`{"artifactId": "theme-sample-<theme>", "kind": "deck", "title": "Aperçu — <theme>",
"file": "<provider>:/deck-asset/<theme>/<sample>"}` — the same provider as the
theme. Do not copy the deck's content: the address is enough.

If the result lists findings, report them; do not change the deck.

## Step 3 — Complete

Call `complete` with, in French: the theme, its description, the number of
slides, and one line: ▶ Présenter, ⧉ Présenter dans une fenêtre et ⎙ PDF
montrent le thème ; « Création d'une présentation » écrit une présentation dans
ce thème.
