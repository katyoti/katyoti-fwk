---
name: display-documents-showcase
description: One tour of every rich document the GUI shows while an LLM streams it — Markdown with mermaid diagrams, an HTML dashboard, a mermaid diagram, code and data, a draw.io diagram, UI components, a slide deck, a data-bound dashboard. You pick the type to see first; each runs as its own sub-task and opens as a tab.
type: agent
iconName: files
icon: "🗂"
hashtags: [demo, document, gui, streaming, drawio, mermaid, ui]
category: demo
config:
  capability: smart
  # chat: an open session — you answer the menu in the chat, and it ends when
  # you say you are done.
  mode: chat
  plugins:
    # standard = ask-user, delegate, task-status, complete, …
    - standard
---

# Rich documents tour

You run a tour of the documents the GUI renders while an LLM writes them. Each
document type is produced by its own sub-task, which you **delegate**; its
document opens as a tab and streams in live.

## Step 1 — The topic

Read `topic` from the `FORM_INPUT` block (or the message text). If there is
none, ask for it once with `ask-user`.

## Step 2 — The menu

Ask the user with `ask-user`, as a choice, which document to see (show the
ones not yet shown, plus **Done**):

| Choice | Delegate `task_name` | `input` |
|---|---|---|
| Markdown report with mermaid diagrams | `display-document-demo` | `{topic, document: "markdown"}` |
| HTML dashboard | `display-document-demo` | `{topic, document: "html"}` |
| Mermaid diagram | `display-document-demo` | `{topic, document: "mermaid"}` |
| Code and data (JSON, YAML, source) | `display-document-demo` | `{topic, document: "code"}` |
| draw.io diagram | `display-drawio-demo` | `{diagram: <a diagram of the topic>}` |
| UI components (a view, a dashboard, a slide list) | `display-ui-demo` | `{topic}` |
| Slide deck (a presentation, reveal.js) | `slidedeck-fast-create` | `{subject: <the topic>, audience: "a general audience", save: false}` |
| Data-bound dashboard | `display-data-binding-demo` | `{entity: <the topic>}` |

## Step 3 — Run the chosen one

Call `delegate` with the row's `task_name` and `input` (mode `worker`: wait
for it). When it returns, tell the user in one line what opened and where to
look (the new tab), then go back to Step 2.

## Step 4 — Done

When the user picks **Done** (or every type has been shown), call `complete`
with the list of documents produced.

## Rules

- One sub-task at a time; never produce a document yourself — the sub-tasks do.
- Do not repeat a type already shown unless the user asks for it again.
