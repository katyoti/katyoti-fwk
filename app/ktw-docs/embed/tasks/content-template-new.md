---
name: content-template-new
title: "Nouvelle trame"
description: "Crée une trame de livrable dans le module de cet objet — en partant d'une trame existante (comité d'architecture, proposition de solution…) et en la modifiant comme demandé : une partie en plus, en moins, dans un autre ordre, d'autres sources. Elle est ensuite proposée en premier dans les listes « Trame »."
type: task
kind: object
# Where a template makes sense: a module (a documentation, code or system
# module), a document and a template to start from - never
# every object (an information-source, a goal…). A domain whose own types
# want it declares it by listing them on its capability.
types: [module, system-module, software-module, documentation, document, content-template]
iconName: list-details
icon: "🧩"
hashtags: [template, deliverable, deck, document]
category: documents
config:
  # A conversation: the template is made, then adjusted on request.
  mode: chat
  capability: smart
  plugins:
    - standard        # list-resources / read-resource: the templates in reach
    - cwm             # cwm-object-list / -get / -create
    - object-context  # the object the action was run on: its module
---

# Nouvelle trame

You create a **content template** object in the module of the current
object (its id is the `object="…"` attribute of the `<cwm_object_context>`
block; its module is the id's prefix), copied from `base` and changed as
`changes` asks. Read the values from the `FORM_INPUT` block.

A content template says WHAT a deliverable must say, section by section; it
never holds the deliverable's text. Its declaration (the data-type
`ktw-docs:/data-type/content-template`):
- `name` (kebab-case), `label`, `description`, `purpose`, `target` (an object
  type, or none), `partial` (a section group, only included);
- `sections`, in reading order, each: `id` (kebab-case, unique), `title`,
  `intent` (what it must establish), `sources` (each `{from, value, note}`,
  `from` one of `field`, `children`, `named-file`, `files`, `links`,
  `object`, `form`), `guidance`, `required`, `repeat` (an object type: one
  section per child), `weight` (1–5), `deck: {layout}`, `doc: {level}`;
- or a section `{id, include: <a partial template's name>}` and nothing else.

## Step 1 — Read the base

The templates a sidecar ships (ktw-docs': `architecture-board`,
`solution-proposal`, `steering-committee`, `overview` and the partial
`context-and-scope`, `options-and-recommendation`, `risks-and-mitigations`,
`next-steps`) are **contributions, not CWM objects**: no `cwm-*` tool finds
them. Read `base`:
1. `cwm-object-list` with type `content-template`: a project template whose
   `content-template` capability has `name: <base>` comes first.
2. Else `list-resources` with kind `content-template`, then `read-resource`
   on the URI of the one named `base` (`<provider>:/content-template/<base>`).
3. Read each partial template it includes the same way, to know its parts.
Copy the base AS IT IS — its sections, sources, guidance, includes — and
change only what Step 2 says. If `base` is found nowhere, say so and stop:
never write a template from scratch, never invent a source.

## Step 2 — Change it

Apply `changes` and nothing else: a new section gets an `id`, a `title`, an
`intent` and, when the request says where its facts are, its `sources`; a
removed section goes; a moved one moves. Set `name` and `label` from the
form, keep `description`, `purpose` and `target` unless the change says
otherwise. Never invent a source the request does not name: a new section
without one has none.

## Step 3 — Store it

`cwm-object-create` in the module: type `content-template`, `name` = the
label, capability `content-template` = the template (the whole declaration).
A refusal names what does not fit the data-type: fix it and create again.

## Step 4 — Hand over, then keep going

Tell the user the new object's id, its sections in order (marking the ones
you added or changed, and naming the parts that come from included
templates), and where it is offered: first in the « Trame » list of
« Présentation du dossier » and of « Création d'une présentation ». Then ask
what to adjust; each adjustment is a `cwm-object-update` of the template (`cwm-object-get` it
first and pass the `revision` it gave).
