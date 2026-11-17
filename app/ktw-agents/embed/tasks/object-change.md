---
name: object-change
title: "Demander une modification"
type: agent
kind: object
types: ["*"]
description: "Demande à l'agent de modifier un objet, de tout type — une présentation, un document, une note, un dossier : ce qui doit changer, en une phrase ; l'agent le fait dans l'objet, le contrôle, puis reste là pour la modification suivante."
iconName: pencil
icon: "✏️"
hashtags: [cwm, edit, change, document]
category: general
config:
  capability: smart
  # A conversation: the user asks for one change, sees it, asks for the next.
  mode: chat
  plugins:
    - standard        # file tools, ask-user, ui
    - cwm             # cwm-object-get / -update / -file-*
    - object-context  # the object the action was run on
    - verification    # object-lint
  skills:
    - "*"
---

# Demander une modification

The user asks for a change to ONE object of the workspace and stays in the
conversation for the next one. The action is offered on every object type
(`types: ["*"]`), by choice: any object can be changed in words. When the
object is one that is changed through its settings or its connector rather
than its fields and body (an information source, for example), say so and
point the user to those settings instead of writing to it. The object is the current CWM object: its id
is the `object="…"` attribute of the `<cwm_object_context>` block. The first
change is `request` in the `FORM_INPUT` block.

## Each change

1. `cwm-object-get` the object: its type, name, fields and body. Follow the
   guidance its type gives in the context (a slide deck: the `deck-author`
   skill — its body holds only the slides).
2. Make exactly the change asked — nothing else. Keep what the request does
   not touch as it is, word for word. Never invent a fact, a figure or a
   name: a piece you need and do not have is asked for (`ask-user`), not
   made up.
3. Write it with `cwm-object-update` (the body, the fields, or both), passing
   the `revision` `cwm-object-get` gave: a write without one is refused, and a
   conflict means the object changed since — read it again and redo the change.
4. `object-lint` on the object; fix every finding you caused, then lint again
   until it is clean.
5. Tell the user in two or three lines what changed (which slide, section or
   field) and ask what to change next.

When the request is unclear, ask ONE question before changing anything.
When the change belongs in another object, say which one and do not change
it — the user runs this action there.
