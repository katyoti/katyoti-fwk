# ktw-compose templates

The canonical templates ktw-compose stamps when scaffolding files-only
sidecars and authoring artifacts. **Single source of truth** (drift clause,
audit F5): any change to a canonical pattern updates the matching template in
the same commit; the engine's integration test builds a rendered sidecar so
rot fails CI loudly.

Engine: `internal/extensions/compose/templates` — strict `{{key}}`
substitution (NOT text/template, per Q-A3): templates stay copy-pastable
standalone examples. Unresolved placeholders are an error.

| Template | Produces | Vars |
|---|---|---|
| `sidecar/main.go.tmpl` | `app/<name>/main.go` | name |
| `sidecar/embed.go.tmpl` | `app/<name>/embed.go` | includes |
| `sidecar/cmd/root.go.tmpl` | `app/<name>/cmd/root.go` | name |
| `sidecar/cmd/bootstrap.go.tmpl` | `app/<name>/cmd/bootstrap.go` | name, short, long, ktsp_description, options (pre-assembled `sidecarkit.WithX(),` lines) |
| `sidecar/makefile-note.txt.tmpl` | instructions (the Makefile uses one APPS list — no per-binary target) | name |
| `skill.md.tmpl` | a strict-9-key skill file | name, description, hashtags, category, advertised, body |
| `task.md.tmpl` | a flat task file (modern shape — config under `config:`) | name, type, description, config, body |
| `persona.md.tmpl` | a persona (TASK-shaped, `category: persona` + binding fields) | name, description, bind_mode, types, skills, config, body |
| `reference.md.tmpl` | a reference doc | title, body |

Docs: `modules/compose/templates.md`.
