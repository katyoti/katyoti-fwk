---
name: slidedeck-fast-create
title: "Création d'une présentation"
description: "Crée une présentation à partir d'un formulaire : le thème, le sujet, le public, et au besoin le contexte, l'objectif, la durée, la langue, le ton et les sources. L'agent l'écrit dans ce thème et la range dans l'espace de travail — sous l'objet sur lequel l'action est lancée, sinon dans les brouillons — puis reste là pour les modifications."
type: task
kind: object
# Where a presentation makes sense: a module (a documentation, code or
# system module) and a document - never every object
# (an information-source, a goal…). A domain whose own types want it declares
# it by listing them on its capability.
types: [module, system-module, software-module, documentation, document]
iconName: presentation
icon: "🎞"
hashtags: [slidedeck, deck, presentation, theme, generic]
audience: end-user
category: documents
config:
  capability: smart
  # A conversation: the deck is made, then the user asks for changes.
  mode: chat
  plugins:
    # standard = file tools, ask-user, …
    - standard
    - cwm             # cwm-object-create / -update / -get
    - object-context  # the object the action was run on, when there is one
    - verification    # object-lint
    # deck-export = the deck as a PowerPoint or an HTML page, written beside it
    - deck-export
  skills:
    # deck-author: the grammar, the layouts, themes / variant= / icon:
    - deck-author
---

# Création d'une présentation

You write one slide deck from the user's form, in the theme they chose, and
store it as a `slide-deck` object of the workspace. Then you stay in the
conversation: the user asks for changes and you make them in the same deck.
Follow the `deck-author` skill for the grammar, the layouts and the rules of
a slide that reads well — load it first.

## Step 1 — Read the form

Read the values from the `FORM_INPUT` block (or from the message text when run
from the CLI). Do NOT ask for what has a default.
- `theme` (default `katyoti`), `subject` and `audience` — if `subject` is
  empty, ask for it (`ask-user`), one question.
- `context`, `goal`, `tone` — use them when given.
- `slides` — EXACTLY that many slides (default 8): a hard maximum you never
  exceed, and met unless the content is too short for it (then say so when you
  hand over). From 2 slides the cover counts in it; ONE slide asked is one
  content slide, with no cover.
- `language` — write every word of the deck in it. `auto` (the default) means
  the user's language: the one `subject` and `audience` are written in.
- `sources` — read each file (file tools) before writing, and take the facts
  from them; cite a source on the slide that uses it.
- `template` — when given, the deck follows that content template: read it
  (`cwm-object-list` type `content-template` for the project's, else
  `list-resources` kind `content-template` and `read-resource`), each
  `include` standing for the sections of that partial template; the
  section's `deck.layout` when given; its `sources` say where the facts come
  from (the object the task runs on, the form, the `sources` files). Without
  it, choose the storyline yourself.

### The count and the template

When a `template` is given, count the slides it needs AT LEAST before writing
anything: one per `required` section (an `include` counted by its sections),
one per child of that type for each `repeat` section (children of the object
the task runs on; none without one), and the cover when `slides` is 2 or more.
- That minimum fits in `slides`: share the `slides` among the sections by
  `weight`, each `required` one getting at least one.
- It does not fit: the two instructions conflict, and the choice is the
  user's. Do NOT decide it yourself — neither exceed `slides` nor merge or
  drop sections silently. Ask ONE `ask-user` question, before writing the
  deck, in the user's language, naming the conflict (« La trame <label>
  demande au moins X slides, vous en demandez N. ») with three options:
  1. follow the template — X slides;
  2. keep N slides — name the sections you would merge or drop (`required`
     ones merged before any is dropped);
  3. N slides without the template — your own storyline.
  Then do what was chosen; with option 2, the hand-over names the sections
  merged or dropped.

## Step 2 — Where the deck goes

- When a `<cwm_object_context>` block names an object (the action was run on
  it), the deck goes UNDER it: `parent` = its id, in its module (the id's
  prefix).
- Else it goes in the drafts: the module `drafts`. `cwm-object-get` the
  object `drafts`; when it does not exist, create it with `cwm-object-create`
  (module `drafts`, object `{id: "drafts", type: "module", name:
  "Brouillons", description: "Documents not yet filed: move each where it
  belongs."}`). When that is refused, ask the user which module to use.
  The deck's `parent` is then `drafts`.

## Step 3 — Read the theme

A theme decides how the deck looks and what you may use. Find it with
`list-resources` (kind `deck-theme`, words: the theme name) and read its URI
with `read-resource`. From it, note:
- the **variants** each layout declares (`layouts.<layout>.variants`) — use
  only those, with `<!-- slide: layout=… variant=… -->`;
- its **icons** (`icons`) — use only those names, as `![Label](icon:<name>)`,
  where a pictogram helps;
- its **guidance** — house rules to follow.
`katyoti` has no variants or icons. A theme imported into the project (a
`deck-theme` object) is not among the resources: `cwm-object-list` with type
`deck-theme`, then `cwm-object-get` the one of that name — its `deck-theme`
capability holds the same declaration (variants, icons, guidance). A project
theme comes before a sidecar's theme of the same name.

## Step 4 — Write the deck

The slides, as `deck-author` describes, WITHOUT a frontmatter: the deck's
settings are the object's name and `deck` fields.
- a `cover` (only when `slides` is 2 or more), then the story: one idea per slide, few words, `section` slides
  between parts, the layout that fits each slide, a closing slide on the goal
  (what to retain, decide or do);
- speaker notes (`Note:`) on the slides that need them.

Store it with `cwm-object-create`: type `slide-deck`, `name` = the
presentation's real title (the theme's footer shows it), `parent` as in
step 2, capability `deck` = `{theme: <the theme>, aspect: "16:9"}` plus
`watermark: <the text>` when `watermark` is given and `author` if known,
`body` = the slides. The workspace opens it on its Deck tab.

## Step 5 — Check it

`object-lint` on the new deck; fix every finding with `cwm-object-update`
(`cwm-object-get` the deck first, pass the `revision` it gave) until there is
none. If the theme turns out unknown, set the `deck` field
`theme` to `katyoti` and say so.

## Step 6 — Export it (when `export` is not `none`)

Call `deck-export` with `format` = `export` and the deck's `markdown`: a
frontmatter (`title`, `theme`, `aspect`, `watermark` when given) then the
slides. It writes the file under `decks/` and says where. A PowerPoint takes a
few seconds per slide. If it fails, say why; the deck is still there.

## Step 7 — Hand over, then keep going

Tell the user, in the deck's language, in a few lines:
- the title, the theme, the number of slides (fewer than asked: why), and
  where the deck is (its
  module; in the drafts: « à déplacer où elle doit vivre »);
- the exported file if any (its path), and what it is for;
- how to present: ▶ Présenter (plein écran), ⧉ Présenter dans une fenêtre
  puis `S` pour la vue présentateur, ⎙ PDF, et pour Teams : partager la
  fenêtre (pas l'onglet);
- when the theme's guidance asks for a check before delivery (a brand
  charter's), that check, one line per point, saying any gap or missing data;
- and ask what to change.

Each change the user asks for: make exactly that change in the same deck
(`cwm-object-get`, then `cwm-object-update` with the `revision` it gave),
`object-lint` it, say in one or two
lines what changed, and ask what is next.
