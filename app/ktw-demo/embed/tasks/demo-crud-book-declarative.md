---
name: demo-crud-book-declarative
description: Book table with declarative row-click (Mode B) — clicking a row opens the detail view instantly, with NO agent round-trip
type: task
iconName: bolt
icon: "⚡"
hashtags: [demo, crud]
category: demo
config:
  capability: fast
  mode: chat
  plugins:
    - standard
    - records
---

# Book CRUD Demo (Mode B — declarative shortcut)

You mount a Book table whose row-click is handled by the HOST
(declarative `actions` shortcut) — the click never reaches you.
This demonstrates the latency path: the detail view opens instantly
even while you are idle.

Records live on the `memory` CRUD connection:
- collection: `memory:/book` — record: `memory:/book/<id>`
- `SCHEMA_URI` = `ktw-demo:/data-type/book` (pass as `schema_uri`
  on every save-record)

`BOOK_TYPES` (the shared types map for every schema below):

```json
{
  "BookList": {
    "type": "array",
    "label": "Books",
    "items": "Book",
    "extensions": [
      { "type": "ui-table", "name": "default",
        "columns": [
          { "id": "title", "label": "Title", "sortable": true },
          { "id": "author_email", "label": "Author" },
          { "id": "rating", "label": "Rating", "align": "center", "width": 80 },
          { "id": "is_finished", "label": "Finished", "align": "center", "width": 100 }
        ],
        "striped": true, "highlightOnHover": true }
    ]
  },
  "Book": {
    "type": "object",
    "label": "Book",
    "idField": ["id"],
    "titleField": ["title"],
    "fields": {
      "id": { "type": "string", "label": "ID", "optional": true },
      "title": { "type": "string", "label": "Title" },
      "author_email": { "type": "string", "label": "Author email", "content": { "type": "email" }, "optional": true },
      "rating": { "type": "number", "label": "Rating", "content": { "type": "rating", "max": 5 }, "default": 3 },
      "progress_pct": { "type": "number", "label": "Progress (%)", "content": { "type": "slider", "min": 0, "max": 100 }, "default": 0 },
      "is_finished": { "type": "boolean", "label": "Finished?", "default": false },
      "created_at": { "type": "string", "label": "Created at", "content": { "type": "date-time" }, "optional": true }
    },
    "extensions": [
      { "type": "ui-view", "name": "default", "layout": "detail-pane",
        "groups": [
          { "title": "Bibliographic", "cols": 2, "fields": ["title", "author_email"] },
          { "title": "Reading status", "cols": 2, "fields": ["rating", "progress_pct", "is_finished"] },
          { "title": "Audit", "cols": 2, "fields": ["id", "created_at"] }
        ] }
    ]
  }
}
```

## Steps — do ALL of this immediately, no questions

1. `list-records` with `uri: "memory:/book"`. If `count` < 3, seed
   3 books via `save-record` (`uri: "memory:/book"`,
   `schema_uri: SCHEMA_URI`) — reuse the seed data from
   demo-crud-book (Pragmatic Programmer / Clean Code / DDIA), then
   `list-records` again for rows with ids.
2. Mount the table WITH the declarative click shortcut:
   `display-ui-component` with:
   - `artifactId: "book-table-declarative"`, `kind: "table"`,
     `persistent: true`, `title: "Books (declarative)"`
   - `schema`: `{"name": "book", "version": 1, "root": "BookList", "types": BOOK_TYPES}`
   - `data`: the records array
   - `actions`:
     ```json
     {
       "click": {
         "tool": "display-ui-component",
         "argsTemplate": {
           "artifactId": "book-detail-{{item.id}}",
           "kind": "view",
           "mode": "view",
           "title": "{{item.title}}",
           "schema": {"name": "book", "version": 1, "root": "Book", "types": BOOK_TYPES},
           "data": "{{item}}"
         }
       }
     }
     ```
3. Tell the user (one short message): "Row clicks are handled by
   the host — click any row and its detail view opens instantly,
   without me in the loop. Tell me when you want to add / edit /
   delete instead." Then `ask-user` ("Click rows freely — they
   don't reach me. Anything else: add a book / edit <title> /
   delete <title> / done.").
4. Text commands (add / edit / delete) work exactly as in
   demo-crud-book (save-record / delete-record on `memory:/book`,
   confirm deletes, refresh the table after every change — same
   `artifactId` so the actions shortcut survives the re-mount).
   Row clicks will NEVER arrive as ui_event — don't wait for them.
