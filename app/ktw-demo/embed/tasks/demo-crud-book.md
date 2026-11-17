---
name: demo-crud-book
description: Schema-driven Book CRUD — table, view, edit form, save and delete, with the agent driving every action (Mode A)
type: task
iconName: books
icon: "📚"
hashtags: [demo, crud]
category: demo
config:
  capability: fast
  # mode: chat = interactive session semantics: the `complete` tool is
  # filtered from the catalog, text-only turns loop back with an
  # ask-user nudge, and ask-user prompts route direct to the chat.
  mode: chat
  plugins:
    - standard
    - records
---

# Book CRUD Demo (Mode A — agent-mediated)

You drive a schema-driven CRUD surface for **Book** records. Every
user interaction (row click, form submit) comes back to you, and you
decide the next step using the record tools.

## Constants

Records live on the `memory` CRUD connection (in-process record
store — resets when the universe core restarts, perfect for a demo).
Record addressing is `<connection>:/<type>[/<id>]`:

- `COLLECTION_URI` = `memory:/book` (list + create here)
- a record's URI = `memory:/book/<id>` (load / update / delete)
- `SCHEMA_URI` = `ktw-demo:/data-type/book` (pass as `schema_uri`
  on every save-record so values are validated before persisting)
- `SCHEMA` (inline AST — use verbatim in `schema`; switch the
  `root` field per mount as instructed below):

```json
{
  "name": "book",
  "version": 1,
  "root": "BookList",
  "types": {
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
            { "id": "progress_pct", "label": "Progress", "align": "right", "width": 100 },
            { "id": "is_finished", "label": "Finished", "align": "center", "width": 100 }
          ],
          "pagination": true, "pageSize": 10, "striped": true, "highlightOnHover": true }
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
        "notes": { "type": "string", "label": "Notes", "content": { "type": "markdown" }, "optional": true },
        "rating": { "type": "number", "label": "Rating", "content": { "type": "rating", "max": 5 }, "default": 3 },
        "progress_pct": { "type": "number", "label": "Progress (%)", "content": { "type": "slider", "min": 0, "max": 100 }, "default": 0 },
        "is_finished": { "type": "boolean", "label": "Finished?", "default": false },
        "created_at": { "type": "string", "label": "Created at", "content": { "type": "date-time" }, "optional": true }
      },
      "extensions": [
        { "type": "ui-form", "name": "default", "layout": "groups",
          "submit_label": "Save book", "cancel_label": "Discard",
          "groups": [
            { "title": "Bibliographic", "cols": 2, "fields": ["title", "author_email"] },
            { "title": "Reading status", "cols": 2, "fields": ["rating", "progress_pct", "is_finished"] },
            { "title": "Notes", "cols": 1, "fields": ["notes"] }
          ] },
        { "type": "ui-view", "name": "default", "layout": "detail-pane",
          "groups": [
            { "title": "Bibliographic", "cols": 2, "fields": ["title", "author_email"] },
            { "title": "Reading status", "cols": 2, "fields": ["rating", "progress_pct", "is_finished"] },
            { "title": "Notes", "cols": 1, "fields": ["notes"] },
            { "title": "Audit", "cols": 2, "fields": ["id", "created_at"] }
          ] }
      ]
    }
  }
}
```

## Step 1 — Seed + show the table

Do ALL of step 1 immediately and autonomously — do NOT call
`ask-user` until the table is mounted (step 1.3 done).

1. Call `list-records` with `uri = "memory:/book"`.
2. If `count` < 3, IMMEDIATELY seed with `save-record`
   (`uri: "memory:/book"`, `schema_uri: SCHEMA_URI`, `value: {...}`)
   until 3 books exist — do not ask permission, seeding is part of
   the task. Seed data:
   - `{title: "The Pragmatic Programmer", author_email: "thomas.hunt@example.com", rating: 5, progress_pct: 100, is_finished: true, created_at: "<now ISO 8601>"}`
   - `{title: "Clean Code", author_email: "robert.martin@example.com", rating: 4, progress_pct: 60, is_finished: false, created_at: "<now ISO 8601>"}`
   - `{title: "Designing Data-Intensive Applications", author_email: "martin.kleppmann@example.com", rating: 5, progress_pct: 30, is_finished: false, created_at: "<now ISO 8601>"}`
   Then `list-records` (`uri: "memory:/book"`) again to get the
   rows WITH their ids.
3. Mount the table:
   `display-ui-component` with `artifactId: "book-table"`,
   `kind: "table"`, `schema: SCHEMA` (root = `BookList`),
   `data: <the records array from list-records>`,
   `persistent: true`, `title: "Books"`.

## Step 2 — The interaction loop

After mounting, call `ask-user` (text):
"The book table is open. Click a row to view it, or tell me:
edit <title> / delete <title> / add a book / done."

The answer is EITHER plain text OR a JSON envelope from a UI
interaction:

```json
{"ui_event": {"artifact_id": "book-table", "event": "click", "value": {"item": { ...the clicked row... }}}}
```

Handle each case, then ALWAYS return to `ask-user`. `ask-user`
MUST always carry a non-empty `text` question — never call it with
empty input.

UI events can also arrive from the DETAIL VIEW's built-in action
buttons (`artifact_id: "book-detail"`):

- `event: "edit"` → mount the edit form for `value.item` (same as
  the Edit flow below).
- `event: "delete"` → run the Delete flow below (confirm first).
- `event: "close"` → the user closed the view; just re-ask. If the
  SAME event arrives twice in a row (stale buffered click), ignore
  the duplicate and re-ask.

Main cases:

- **Row click** (`event: "click"`): take `value.item.id` →
  `load-record` (`uri: "memory:/book/<id>"`) → mount the detail view:
  `display-ui-component {artifactId: "book-detail", kind: "view",
  schema: SCHEMA with "root": "Book", data: <the
  record>, persistent: true, title: <book title>}`.
- **Edit <title>** (text): find the record id from your last
  `list-records`, `load-record` (`uri: "memory:/book/<id>"`), then
  mount the edit form:
  `display-ui-component {artifactId: "book-edit", kind: "form",
  mode: "edit", schema: SCHEMA with "root": "Book",
  data: <the record>, persistent: true}`. Tell the user to
  edit and press "Save book".
- **Form submit** (`event: "submit"` on `book-edit`): call
  `save-record` with `uri: "memory:/book/<id>"`,
  `schema_uri: SCHEMA_URI`, and `value = the submitted data`. If
  the output has `violations`, tell the user exactly which fields
  to fix and keep the form open. On success, `list-records` and
  re-mount `book-table` with the fresh rows.
- **Add a book** (text): mount an EMPTY create form
  (`artifactId: "book-new", kind: "form", mode: "create",
  schema root Book, persistent: true`). On its `submit`
  event call `save-record` with the COLLECTION uri
  (`"memory:/book"`, server assigns the id), then refresh the
  table.
- **Delete <title>** (text): confirm first via `ask-user`
  ("Delete '<title>'? yes/no"). On yes: `delete-record`
  (`uri: "memory:/book/<id>"`), then refresh the table. On no:
  back to the loop.
- **done**: say goodbye; the session stays open (this task runs
  `mode: chat`) so the user can continue chatting.

## Rules

- NEVER invent record ids — they come from `list-records` /
  `save-record` outputs.
- After EVERY successful save or delete: first `list-records`,
  then re-mount the table (same `artifactId: "book-table"`) with
  the fresh rows, THEN `ask-user`. Never skip the re-mount.
- Validation failures are NOT errors — relay the per-field
  violations conversationally and let the user fix them.
- NEVER end the conversation on your own — after every action,
  return to `ask-user`. Only the user decides when it's done.
