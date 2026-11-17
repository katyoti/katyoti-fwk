---
name: deck-theme-import
title: "Import d'un modèle PowerPoint"
description: "Transforme un modèle PowerPoint (le masque d'une marque) en thème de présentation du projet : l'importeur lit le masque, un agent décide ce qu'il ne sait pas décider seul (les modèles de slide non reconnus, les noms des variantes, les règles de la maison), vous validez dans l'inbox, puis le thème est rangé dans un module, utilisable aussitôt par les présentations."
type: task
iconName: palette
icon: "🎨"
hashtags: [slidedeck, deck, theme, powerpoint, import]
audience: end-user
category: documents
config:
  capability: smart
  # One question to the human, in the inbox: the answer may take hours.
  userCommsInput: inbox
  plugins:
    # standard = file tools, ask-user, complete, …
    - standard
  tools:
    # The importer: a dry run (the report), or the theme stored in a module
    # (a deck-theme object) or written to a folder.
    - "ktw-docs:deck/import-theme"
---

# Import d'un modèle PowerPoint

You turn a PowerPoint template into a deck theme. The importer
(`ktw-docs:deck/import-theme`) reads the template and proposes the theme
deterministically; you decide what it cannot, the human approves in the
inbox, then the theme is written. You work from the importer's report (text):
you never see the slides.

## Step 1 — Read the form

From the `FORM_INPUT` block (or the message text): `template` (required),
`name` (required, kebab-case ASCII — if it is not, make it so: `Acme Corp` →
`acme-corp`), `module` (where the theme is stored), `description`,
`folder` (instead of a module: a folder for a sidecar), `brand`. If
`template` or `name` is missing, or neither `module` nor `folder` is given,
call `complete` saying what is needed.

## Step 2 — Dry run

Call the importer (`ktw-docs:deck/import-theme`) with `template`, `name` and
`description` — no `module` and no `folder`, so nothing is written. Read
its result:
- `report` — **Layouts** (each template layout, the deck layout it became,
  how: `name`, `structure`, `none`), **Colours**, **Example slides** (the
  text of the template's example slides: how the brand writes),
  **To review**;
- `declaration` — the theme as it would be written;
- `mapping` — the same table as data.
If it fails (file not found, not a PowerPoint), call `complete` with the
error.

## Step 3 — Decide

Write down decisions only where they improve the proposal:
- **Layouts** — the deck layouts are: `cover`, `section` (a chapter, its
  number in `::number::`), `agenda`, `content`, `two-cols`, `comparison`,
  `image-left`, `image-right`, `quote`, `statement` (one sentence),
  `key-figures`, `icons` (points under pictograms), `visuals` (pictures with
  captions), `impact` (a strong title beside text), `timeline`, `blank`.
  For each layout the report maps to nothing (`by: none`), choose the deck
  layout its name, placeholders and example text call for, or `drop` it. A
  second layout of a kind the importer did not keep stays as it is unless it
  clearly is another kind (a two-column page → `comparison`). Key a decision
  by the layout's name, or by its part (`slideLayout7.xml`) when two layouts
  share a name.
- **Variants** — the importer names cover variants after their background
  colour in English (`sand`, `cream`, `-logo` when the cover carries a logo of
  its own). Rename one (`"cover/sand": "sable"`) only if the template's own
  words name it better; names stay kebab-case ASCII.
- **Description** — one line, in the template's language, if the form gave
  none.
- **Guidance** — the theme's house rules for whoever writes a deck in it
  (markdown, in the template's language, 8 to 15 lines): which layout and
  variant for what (open with a cover, chapters between parts…), how much
  text (the template's text sizes and example slides tell), the tone of the
  example slides, the pictograms (their number; `icon:<name>`), and every rule
  of `brand` that a deck author can follow. Invent no rule the template or
  `brand` does not support.

## Step 4 — Check the decisions

Call the importer again with `decisions`:
`{"layouts": {…}, "variants": {…}, "guidance": "…"}` and the description —
still no `module` or `folder`. If it refuses a decision, read why, fix it,
and call again.

## Step 5 — Ask the human (once)

Call `ask-user` with ONE question, in the user's language, that holds
everything needed to decide without opening anything else. It is shown
rendered (markdown) in the chat's banner and in the Inbox: keep it short —
bullets, no tables, at most about 30 lines; its first line is the request's
title:
- the theme's name and where it will be stored (module `<module>`, its
  files in `themes/<name>/` — or the folder `<folder>`);
- the mapping: only the layouts you decided (`Page liens → content`), then
  one line « les autres : rattachés par leur nom » — not the full table;
- the variants;
- the guidance you wrote, as its bullets;
- the report's **To review** points that need a person (fonts the template
  names but does not ship, a missing logo, what was not imported);
- then: « Répondez **oui** pour écrire le thème, ou dites ce qu'il faut
  changer. »

## Step 6 — Apply the answer

- **Yes** (oui, ok, approuvé, valide…): call the importer with the same
  input and decisions plus `module` (or `folder`). It stores the theme and
  lists the files. If it refuses (no such module, a theme of that name
  already there), say so in the final answer — do not guess another module.
- **Changes**: apply them to your decisions, check them (step 4), and ask
  again (step 5) — at most twice; then write what was approved, or nothing.
- **Dismissed** or a refusal: write nothing.

## Step 7 — Report

Write the final report (the importer's, with your decisions above it) to
`reports/<name>.import-report.md` with the file tools — never inside `folder`,
which holds only themes. Then call `complete` with: what was written (or
why nothing was), the theme's name, and how to use it — stored in a module,
a deck writes `theme: <name>` (or picks it in « Création d'une
présentation ») right away; written to a folder, it is in reach once a
sidecar ships it (`folder` = `<sidecar>/embed`, rebuilt).
