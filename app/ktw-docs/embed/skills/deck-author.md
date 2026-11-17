---
name: deck-author
description: >-
  Write a presentation as a markdown slide deck shown live with reveal.js — load
  whenever the user asks for slides, a deck, a presentation or a talk.
hashtags: [slides, deck, presentation, revealjs, markdown]
category: writing
icon: 🎞
iconName: presentation
---

# Writing a slide deck

A presentation is **one markdown file**. You write it; the GUI shows it live with
reveal.js — navigation, full screen, drawing on slides, a speaker view, PDF. You
never write HTML, never build a page of cards, never build a PowerPoint by
hand — a deck leaves the tool as a PowerPoint or a page through its export.

## 1. Show it

Call `display-ui-component`:

```json
{ "artifactId": "deck-<topic>", "kind": "deck", "title": "<title>", "data": "<the markdown>" }
```

- Slides appear while the call streams.
- The result carries `findings`: each is a problem at a line. Fix every one at
  its line and call `display-ui-component` again with the **same**
  `artifactId` — the open deck updates in place, on the slide being shown.
- Do not set `persistent`.
- To change **one slide** of a long deck without resending it, use
  `patch-ui-component` with the slide's number, from 1:
  `{"op": "replace", "target": "slide-3", "value": "<the slide's markdown>"}`
  (also `add` before a slide, `remove`, and `append` with `target: "slides"`).
  A patch is not linted; display the whole deck again when you need findings.
- To show a deck that is already a file, pass `"file": "<path>.deck.md"`
  instead of `data`; you do not resend the markdown.
- To keep the deck, write it with `file-write` to `<folder>/<name>.deck.md` —
  the `.deck.md` extension makes it a deck wherever a file is shown.
  `ktw-docs deck lint <file>` checks it.

### If you are unsure how a deck renders

Optional — the lint stays the check. When you doubt what a slide looks like (a
slide that may be too full, a draw.io or mermaid diagram, a chart), look at it
as a picture:
- a deck file, from a shell: `ktw-docs deck export <file>.deck.md --format png`
  writes one picture per slide, `slide-01.png`, `slide-02.png`… into
  `<file>.slides/` beside it (`--out <folder>` for another); read the ones in
  doubt. It starts its own display server and needs a Chromium browser (Edge,
  Chrome or Chromium, or `KTW_BROWSER`); when one is missing it says what and
  how to provide it — report that, do not retry.
- in a run, a deck object or a deck with no file: the `deck-export` tool with
  format `pptx-pictures` (pass the markdown) writes a PowerPoint of one picture
  per slide, exactly as shown; the pictures are in its `ppt/media/`.

## 2. The grammar

```markdown
---
title: Q4 architecture review
author: Platform team
aspect: "16:9"
---

<!-- slide: layout=cover -->
# Q4 architecture review
One platform, three sidecars

---

<!-- slide: layout=two-cols steps=list -->
## Where we are
- The hub owns the catalog
- Sidecars publish, never read

::right::

![topology](assets/topology.svg)

Note:
Start with the hub; the audience knows the sidecars already.
```

- **Frontmatter** (required): `title` (required), `author` (the exported
  PowerPoint's and HTML page's author; a theme's footer may show it), `theme` (see
  Themes below; `katyoti` when omitted), `aspect` (`"16:9"` or `"4:3"`,
  quoted; the theme's when omitted), `transition`, `watermark`, `mark` (see Themes).
- **Images** (`![Plan](assets/plan.png)`, `background=photos/lac.jpg`) are
  files beside the deck — its folder and below; a path leaving it (`..`) is
  refused, a missing file is a lint warning. A deck **object** may also name
  a file of its module from the module's root: `![Flux](module:/dossiers/d1/travail/flux.drawio)`
  — shown where it lives, so it follows its source.
- **Diagrams** are drawn, not pasted: a ```` ```mermaid ```` block in the slide,
  or a draw.io file beside the deck named as an image
  (`![C4 context](diagrams/context.drawio)`). Put one diagram on its own slide
  (`layout=blank` for a large one) with a short title; never describe in
  bullets what a diagram shows.
- **Charts** from a few numbers: a fenced block ```` ```chart bar ```` (or
  `line`, `pie`), then comma-separated lines — the header names the category
  column and each series, then one line per category (`2024, 120, 40`;
  decimals with a point; a pie has one series). Never invent a number: a
  missing figure is « [DONNÉE À COMPLÉTER] » in the text, not in a chart.
- **Slide groups** — slides a team keeps once and shows in many decks (our
  team, the company context, the closing): a slide whose only setting is
  `<!-- slide: include=<group> -->` shows the group's slides in its place.
  List the groups in reach with `list-resources` (kind `slide-group`) and the
  project's with `cwm-object-list` (type `slide-group`; the name is the
  object's local id). Include a group as it is; never copy a group you can
  include. A group meant to be filled (« [à compléter] » in its slides) is not
  included: copy its slides into the deck and fill them.
- **`---`** alone on a line separates slides; **`--`** puts a slide *below* the
  previous one (a detail the presenter may skip).
- **Settings** go on a slide's **first line**: `<!-- slide: key=value … -->`.
  A settings line anywhere else is ignored — the lint says so. Keys:
  `layout`, `variant` (one look of the layout that the deck's theme declares),
  `background` (a colour like `#0f172a` or an image path),
  `transition` (`none` `fade` `slide` `convex` `concave` `zoom`), `steps`
  (`list` reveals the slide's list items one at a time), `class`. Quote values
  with spaces: `class="dark wide"`. No settings line means `layout=content`.
- **`::name::`** alone on a line starts a slot of the layout.
- **`Note:`** starts the speaker notes; everything after it on the slide is
  notes, seen only in the speaker view.

## 3. Layouts

| Layout | Use it for | Slots |
|---|---|---|
| `cover` | the first slide: `#` title, one subtitle line | — |
| `section` | opening a part: one `##` heading, at most one line | `::number::` (optional, e.g. `02`) |
| `agenda` | the plan: `## Agenda` then an ordered list, `**Part** — line` (2–8) | — |
| `statement` | one strong sentence, very large | — |
| `content` (default) | a `##` heading and one block — a list, a paragraph, a table, code | — |
| `two-cols` | two things side by side: text and an image, two lists, a list and code | `::right::` |
| `comparison` | before / after, option A / option B | `::left::` `::right::` (each starts with a `###` naming the side) |
| `image-left` / `image-right` | an image beside text | `::image::` with one `![alt](path)` |
| `quote` | one quotation, large | `::author::` |
| `key-figures` | numbers that tell the story (1–6), beside a heading and a short text | `::figures::` — a list, `- **15 000** caption` |
| `icons` | points side by side, each under a pictogram (2–8) | `::items::` — a list, `- ![x](icon:name) **Title** — caption` |
| `visuals` | pictures with captions (1–6), beside a heading and a short text | `::visuals::` — a list, `- ![alt](path) caption` |
| `impact` | a large impact title, its supporting points beside it | `::right::` (`###` blocks) |
| `timeline` | steps along a line — phases, a roadmap (2–8) | `::steps::` — an ordered list, `**Step** — line` |
| `blank` | a full-slide diagram, image or table | — |

`ktw-docs deck layouts` lists them with their slots; other sidecars may add
layouts. A list layout arranges its items by their count (one row up to three,
2×2 for four, three or four columns beyond) and says how many it takes — the
lint refuses a list outside the range: split the slide or merge items. The markdown before the first slot marker is the layout's main region;
a leading `##` heading becomes the slide's title.

```markdown
<!-- slide: layout=comparison -->
## Storage: before and after

::left::
### Before
- One file per node
- History by copying

::right::
### After
- One YAML per object
- History as events
```

```markdown
<!-- slide: layout=quote -->
> A path is filing, never meaning.

::author::
The module layout decision
```

### Themes

A theme gives the deck a house look — colours, fonts, a logo, rules, a footer
and a slide number on every slide, backgrounds per layout. You never place any
of it: name the theme and write the story.

- `ktw-docs deck themes --all-sidecars` lists the themes, their variants per layout
  (`cover: variant=sand|cream`) and their icons. Use a theme the user names;
  otherwise leave `theme:` out.
- A theme imported into the project (« Import d'un modèle PowerPoint ») is a
  `deck-theme` object of a module, its files in that module's `themes/<name>/`.
  It is used the same way — `theme: <name>` — and comes before a sidecar's
  theme of the same name, but `deck themes` does not list it: read it as an
  object — `cwm-object-list` with type `deck-theme`, then `cwm-object-get`.
  Its `deck-theme` capability is the same declaration a sidecar's theme makes:
  `layouts.<layout>.variants`, `icons`, `guidance`, `aspect`.
- `<!-- slide: layout=cover variant=sand -->` picks one look of a layout. Use
  only a variant the theme lists for that layout — the lint refuses another.
- `![Truck](icon:truck)` shows one of the theme's pictograms, in the theme's
  colour. Use only names the theme lists.
- A theme is drawn for one slide shape (its `aspect`): leave `aspect:` out and
  the deck takes the theme's. Another shape stretches the theme's logo, rules
  and footer — the lint warns.
- The footer shows the deck's `title`: give the presentation's real name.
- `watermark: BROUILLON` (or `INTERNE`, `CONFIDENTIEL`, any short text) puts a
  large translucent text across every slide; `watermark: none` removes the
  theme's. A theme may also show a small production mark (« Une production
  ArchiFactory »): `mark: off` removes it. Add either only when asked.
- A theme may state house rules (its guidance) — follow them.

## 4. A slide that reads well

- **One idea per slide.** If a slide needs two headings, it is two slides.
- **Few words**: a heading and at most 5 bullets of one line each. Details go
  in the notes, not on the slide. An agenda names its parts in a few words
  each (no « — explanation » after them). A slide whose text does not fit is
  shrunk to fit its page, down to 55 %; one that still does not fit is
  flagged « Too much for one slide »: split it.
- **Reveal lists** the presenter talks through: `steps=list`.
- **Structure**: a cover, a `section` slide per part, a closing slide with what
  to do next. 5–12 slides for a talk of 10–20 minutes.
- **Notes on every content slide**: 1–2 sentences the presenter says.
- **Code and tables**: short — 8 lines of code, 4 rows of table. Give a code
  block its language (```` ```go ````, ```` ```yaml ````): it is highlighted in the
  theme's colours. More belongs in
  a document, not a slide.
- **Images** only with a real path; never invent one.
- **A vertical slide** (`--`) for an optional detail or a backup slide.
- Emphasis with `**bold**`, not with colour or capitals.

## 5. Tell the user how to present

Once the deck is shown, tell the user in one or two lines:

- **▶ Present** — full screen; arrows / space to move, `O` overview,
  `.` blackout, **Alt+click** zooms on a detail (again to zoom out), `Esc` to leave.
- **✎ Draw** (`C`), the pen swatches (`X` cycles them), **▭ Board** (`B`), **⌫ Clear**
  (`Del`) — annotate while presenting.
- **⧉ Present in a window** — the deck opens in the browser, for a second
  screen; `S` there opens the speaker view (current and next slide, notes,
  timer). Edits to the deck still reach that window.
- **⎙ PDF** — the print dialog, one page per slide and per reveal step.
- **⇩ Export ▾** — a PowerPoint (editable slides, or one picture per slide)
  or a standalone HTML page that opens with a double click; to save one
  yourself, call the `deck-export` tool.
- **Microsoft Teams** — present in a window and share that *window* (not a
  browser tab); the speaker view stays private.
- **💬 Review** (a deck object) — comments on a slide, replies, resolve;
  « Changes » lists the slides added, changed and removed since the last
  « ✓ Mark reviewed ».

## 6. Address a review

A deck object's review is its named file `review`
(`cwm-object-named-file-read`, name `review`): JSON with `threads` — each
`slide` (its `title`, else its `index` from 0), `comments` (author, text),
`resolved` — and `reviews`, the reviews marked done. Asked to address the
review, read it, change the slides the open threads ask for (each thread's
slide by its title), and tell the user which threads you addressed and which
you left, and why. Never edit the review file: the reviewers resolve their
threads.
