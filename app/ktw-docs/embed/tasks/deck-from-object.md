---
name: deck-from-object
title: "Présentation à partir de cet objet"
description: "Écrit la présentation d'un objet du projet — un dossier d'architecture, une décision, un module… — en suivant une trame (comité d'architecture, proposition de solution, comité de pilotage, présentation générale, ou une trame du projet) : chaque partie tirée des informations de l'objet et de ses schémas, jamais inventée. Une présentation rangée sous l'objet, dans le thème choisi."
type: task
iconName: presentation
icon: "🎞"
hashtags: [slidedeck, deck, dossier, architecture, board]
category: documents
config:
  mode: worker
  capability: smart
  plugins:
    - standard        # list-resources / read-resource: the content templates
    - cwm             # cwm-object-get / -tree / -list / -named-file-* / -file-* / -create / -update
    - object-context  # the object the action was run on
    - workspace       # file-read: a file outside the object's module
    - verification    # object-lint
    - task            # complete
  skills:
    - deck-author
---

# Présentation à partir de cet objet

You write the presentation of one project object — most often an
architecture dossier — following a **content template** (`template`), in
`theme`, and store it as a slide-deck object under it. Everything a slide
states comes from the object and what it holds: **never invent a fact, a
figure, a name or a decision**. A piece you need and do not find is written
« [DONNÉE À COMPLÉTER] » on the slide and listed at the end.

The object is the current CWM object: its id is the `object="…"` attribute
of the `<cwm_object_context>` block. Its module is the id's prefix. Read the
values of the form from the `FORM_INPUT` block.

## Step 1 — Read the template

A content template says WHAT the presentation must say, section by section:
each section's `title`, its `intent` (what it must establish), its `sources`
(where its facts come from, relative to the object), its `guidance`, whether
it is `required`, its `weight` (1–5: its share of the time), `repeat` (one
section per child of that type) and, sometimes, a `deck` hint (`layout`).
It never holds the text: you write it.

A sidecar's templates are contributions, not CWM objects — no `cwm-*` tool
finds them.
1. The project's templates first: `cwm-object-list` with type
   `content-template`; one whose `content-template` capability has
   `name: <template>` is the one.
2. Else the sidecars': `list-resources` (kind `content-template`, words: the
   template's name) and `read-resource` on its URI.
3. A section with `include: <name>` stands for the sections of that partial
   template, read the same way, in its place.
If `template` is found nowhere, say so in your final answer and use
`architecture-board`.

## Step 2 — Gather what each section needs

Read the object first: `cwm-object-get` (name, description, fields, body),
`cwm-object-tree` under it (its children), `cwm-object-named-file-list` (an
`overview` is its synthesis). Then, for each section, its `sources`:
- `field` — the object's field of that name;
- `children` — `cwm-object-get` each child of that type;
- `named-file` — `cwm-object-named-file-read` with that name;
- `files` — the files matching that glob among the module's files
  (`cwm-object-file-list`, `recursive: true`) — a dossier d'architecture's
  documents and diagrams are its module's files;
- `links` — the objects linked by that kind (`cwm-object-get` shows the edges);
- `object` — what the object itself says (its body, status, history);
- `form` — the form's field of that name.
Keep, for each fact you will use, where it came from (an object id or a file
path). A section whose sources say nothing: keep it if it is `required` (with
« [DONNÉE À COMPLÉTER] »), drop it otherwise.

## Step 3 — Plan the slides

One slide per minute: `minutes` slides at most, the cover included — a hard
maximum, never exceeded. Count first the slides the template needs AT LEAST:
the cover, one per `required` section, one per child for each `repeat`
section.
- That minimum fits: share the slides among the sections by their `weight`,
  each `required` one getting at least one.
- It does not fit: the choice is the user's, not yours — neither exceed the
  count nor merge or drop sections silently. Ask ONE `ask-user` question
  before writing the deck, naming the conflict (« La trame <label> demande au
  moins X slides, vous disposez de N minutes. ») with three options: follow
  the template (X slides); keep N slides — naming the sections you would
  merge or drop (`required` ones merged before any is dropped); or N slides
  in your own storyline. Then do what was chosen.

Use the section's
`deck.layout` when it gives one, else the theme's layouts that fit
(`deck-author`): `section` between parts, `key-figures` for figures the
object states, `comparison` for two options, `timeline` for a roadmap,
`statement` for a decision asked.

## Step 4 — Diagrams and figures

- A diagram that is a **file of the module** is shown where it lives:
  `![<label>](module:/<its module-relative path>)` — it follows its source.
- A diagram **outside the module**: copy it
  beside the deck with `cwm-object-file-write` to
  `decks/<deck-slug>/<file name>` and show it as
  `![<label>](<deck-slug>/<file name>)`; say in the final answer that this
  copy does not follow its source.
- One diagram per slide (`layout=blank` for a large one) with a short title;
  never describe in bullets what a diagram shows.
- A simple flow the object describes in words may be a ```` ```mermaid ````
  diagram; numbers the object states may be a ```` ```chart ````
  (deck-author). Never draw what the object does not say.

## Step 5 — Write and check the deck

1. Write the deck in `language` (`auto`: the object's language): the slides
   in the deck-author grammar, WITHOUT a frontmatter. Give every slide that
   states a fact speaker notes ending with `Source : <object id or file>`, and
   `Section : <the section's id>`.
2. `cwm-object-create` in the object's module: type `slide-deck`, `name` =
   the presentation's real title, `parent` = the object's id, capability
   `deck` = `{theme: <theme>, aspect: "16:9"}`, `body` = the slides.
3. `object-lint` on the new deck; fix every finding with `cwm-object-update`
   (`cwm-object-get` the deck first, pass the `revision` it gave) until there
   is none.
   Unsure how a slide renders: deck-author, « If you are unsure how a deck
   renders » (optional).

## Step 6 — Complete

Call `complete` with: the deck's id and title, the template followed, the
number of slides, the diagrams shown (linked or copied), the sections dropped
for lack of sources, the sections merged or dropped to fit the count, and the list of « [DONNÉE À COMPLÉTER] » to fill — what
the architect must still provide.
