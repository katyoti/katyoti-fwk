---
name: translate-contributions
description: "Translate a pack's contributions (types, panels, workflows, gates, guide recipes) into another language: list the gaps, scaffold the .i18n.yaml stubs, fill them, re-check."
advertised: true
hashtags: [i18n, translation, french, english, contributions, pack]
category: development
---

# Translate contributions

A contribution a person reads in the GUI (an object type's label, a panel's
name, a workflow's states, a gate's checklist, a guide recipe's steps) carries
its other languages in a `<name>.i18n.yaml` file beside it. Its kind declares
which texts are translatable; the one translation lint judges the file against
that declaration. Your job: make every such text exist in the target language,
and nothing else.

What is NOT translated: object CONTENT (what people write in objects), task
and skill bodies (agent instructions), ids, names used as keys, URIs, code.

Each kind lists its own translatable texts — an object type its `label`,
`label_plural`, `description`, action and `pane.badges` labels; a workflow its
states and transitions; a gate its requirements; an edge type its `label`,
`inverse_label`, `description`; an object panel only its `name` and
`description` (the texts inside a declared page stay as written — a page that
follows the person's language is built by its source from `language`); a
data-type its fields' `label` and `description`, in `<name>.dsl.i18n.yaml`
beside the `.dsl.yaml`.

## 1. See what is missing

```
ktw-compose i18n <pack-folder> --lang fr --missing
```

`<pack-folder>` is a binary's `embed/` (e.g. `app/ktw-methods/embed`), an
extension's `contrib/embed`, or a pack's `embed/cwm`. The report lists, per
entry, each missing text by its path (`label`, `description`,
`states.draft`, `transitions.approve`, `actions.<label>.label`…) with its
source text. `status: complete` means there is nothing to do. On a compiled
sidecar, pass the executable instead of the folder (report only).

The source language is English unless the pack says otherwise (`source_lang`
in its `embed/pack.yaml`, or in one entry's `.i18n.yaml`): a customer pack
written in French translates INTO English.

## 2. Scaffold the stubs

```
ktw-compose i18n <pack-folder> --lang fr --scaffold
```

Each entry that misses texts gets, in its `<name>.i18n.yaml` (created when
absent, kept as written when present), one line per missing text:

```yaml
fr:
  label: ~ # Deck theme
  states:
    draft: ~ # Draft
```

The comment is the source text. A `~` still reads as missing.

## 3. Fill every stub

Replace each `~` with the translation; delete the comment. Rules:

- Translate the MEANING for a person using the GUI, not word for word; keep
  it as short as the source (a label stays a label).
- Keep VERBATIM what names something the person clicks or types: the names of
  GUI controls quoted in a description, ids, file names, URIs, code in
  backticks, product names (KaTyoti, CWM).
- A guide recipe's step names its controls `{control:<anchor>}` (or
  `{control:<anchor>|<name>}`): copy each placeholder as it is, never a label
  in its place and never in bold; the GUI says the control's label in the
  person's language. The translation names the same controls as the source.
- Keep the terms the platform already uses in that language: read two or
  three translated entries of the same kind in another pack first (for
  French, `app/ktw-agents/embed/cwm/object-types/*.i18n.yaml`) and reuse
  their words (objet, capacité, panneau, demande…).
- French: typographic apostrophe `’`, a space before `:` `;` `?` `!`,
  sentence case for labels.
- Never add a key the report did not list: a text the kind does not declare
  is an error (`translation-unknown`).

## 4. Check

Run step 1 again: the pack must answer `status: complete` and no entry may
carry `errors`. Then lint the whole pack the way its build does:

```
ktw-compose lint-sidecar --exe <the rebuilt sidecar>
```

A shipped pack is also held by the repo's `TestShippedPacks_TranslatedForEveryLanguage`.
