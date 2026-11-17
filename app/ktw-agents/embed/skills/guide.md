---
name: guide
description: >-
  Use FIRST whenever the person asks how to do something in KaTyoti or its GUI
  (how do I, how to, where is, comment faire): find the guide recipe and offer
  to play it.
iconName: map-2
icon: "🧭"
hashtags: [guide, help, recipe, tour, how to]
category: help
advertised: true
---

# Answering "how do I … in KaTyoti?"

The guide holds **recipes**: short step-by-step walks the GUI plays over its own
controls, each ending in a checked result. A recipe with no check is a **tour**.
Answer from a recipe rather than from memory: its steps match the GUI the person
is running, and the GUI can play it for them.

## Words that mean the GUI, not the object model

"Add a capability", "install", "wire", "apply a config-set", "attach a project",
"answer the inbox" are first **GUI actions** with a recipe. Search the recipes
before reading them as CWM terms (an object capability, an object search): if
a recipe matches, answer with it; only if none does, answer in CWM terms.

## Steps

1. **Search.** Call `list-resources` with `kind: guide-recipe` and two or three
   English words for what the person wants (`words: "add capability"`,
   `words: "answer inbox"`), even when they write in French: the recipes'
   search words are English. The best match comes first.
2. **Read.** Call `read-resource` on the best URI. Its steps say what to press,
   in order; its `requires` names the recipes to do first. When the person
   writes in another language than English, also call `read-resource` with
   `layer: translations`: it holds the recipe's texts per language code
   (`fr` → name, description, steps by step id). Answer from those texts; a
   step it leaves out is said in English. A step names a control as
   `{control:<anchor>}` (`ribbon.inbox`, `guide.check`): never show the
   placeholder; say the control in plain words from its anchor (the Inbox
   button of the ribbon, the recipe's Check button).
3. **Answer.** Say in two or three sentences what the recipe does, then list
   its steps briefly. If it requires another recipe the person has not done,
   mention that one first.
4. **Offer to play it.** End your answer with the recipe as a block, exactly:

   ````
   ```guide-recipe
   uri: ktw-hub:/guide-recipe/add-a-capability
   ```
   ````

   The chat shows it as a *Start this recipe* card; pressing it plays the recipe
   in the person's GUI.

## When nothing matches

If `list-resources` finds no recipe, answer from what you know, say that no
guide recipe covers it yet, and suggest **Help & Feedback** in the profile menu,
where every recipe is listed.

## Rules

- Answer in the person's language.
- Offer at most one recipe card per answer: the best one.
- Never invent a recipe URI: only offer one `list-resources` returned.
- A recipe's `check` proves the result; do not claim the task is done because
  the person followed the steps.
