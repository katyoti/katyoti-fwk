---
name: create-sidecar
description: "Scaffold a files-only ktw-* sidecar (binary shell plus an embed/ tree of skills/tasks/references, no Go logic) using the compose tools. Load when a files-only pack needs its own binary identity."
advertised: false
hashtags: [sidecar, authoring, files-only, create-items]
category: guide
---

# Scaffolding a files-only sidecar

A **files-only sidecar** is a standalone `ktw-*` binary whose value lives
entirely in its `embed/` packs — skills, tasks, personas, references, configs —
with **no Go business logic**. Its Go files are generated boilerplate stamped
from the canonical templates; you never write logic in them.

> **Needs Go?** A sidecar that ships **services / domain logic** is the
> code-bearing track (`ktw-code` / `ktw-gocode`), not this skill. The moment
> the bootstrap needs a `WithDomain` service, stop — hand off to the code track.

## 1. Decide it deserves its own binary

A files-only sidecar's value is its `embed/` — prefer adding packs to an
EXISTING sidecar's `embed/` over minting a new binary, unless the pack needs
its own identity (own catalog presence, own universe membership, own owner).
List what exists first by scanning the sidecar inventory — listing is
`ktw-univers`'s (the orchestrator's) surface, not a compose verb.

A pack that needs no binary of its own build at all is a **content folder**
(`ktw-compose create-content <name>`): a folder with a `ktw-host.yaml` and its
content dirs, served by ktw-host with no Go toolchain — its `cwm/` folder takes
every pack kind (object types, capabilities, panels, gates, agent-contexts).

## 2. Scaffold it

Call the **ktw-compose:compose/create-sidecar** tool:

```json
{
  "name": "ktw-mypack",
  "include": "skills,tasks,references",
  "short": "ktw-mypack — my files-only pack"
}
```

This stamps `app/<name>/` from the canonical templates
(`app/ktw-compose/embed/templates/sidecar/` — the single source of truth for
the boilerplate; never copy Go shells from another sidecar by hand):

```
app/<name>/
  main.go            <- entry: cmd.Execute(embeddedFS)
  embed.go           <- //go:embed of exactly the included dirs
  cmd/root.go        <- version + appcore.BootstrapCLI
  cmd/bootstrap.go   <- sidecarkit.NewSidecar(...), NO domain extension
  embed/skills/usage.md   <- the conventional first skill (when skills included)
  embed/<dir>/README.md   <- one per included dir (go:embed needs a file)
```

Rules the tool enforces: in-repo only (run from the repo root), kebab name,
never overwrites an existing `app/<name>/`. Skills are flat
`embed/skills/<name>.md` files; tasks are flat `embed/tasks/<name>.md` files.

## 3. Build it

Follow the tool's returned note: add the name to the repo Makefile's `APPS`
list, then:

```bash
go build -o bin/<name>.exe ./app/<name>
```

(Windows: always the `.exe` suffix — a stale extension-less twin shadows the
new binary.)

## 4. Put it in a universe

- New solution around it → the **ktw-compose:compose/apply-universe** tool
  (or the interactive `compose-universe` verb) creates the universe and adds
  every member in one pass.
- Existing universe → the **ktw-compose:compose/add-member** tool
  with `{"universe": "<u>", "sidecar": "<name>", "binary": "<abs path>"}`.
  The written row follows the bus convention (transport nats,
  args `sc ktsp nats`).

Verify via `ktw-univers` (the universe read surface) — reading is not a
compose verb.

## 5. Fill the packs

Stub then author — emit skeletons with **ktw-compose:compose/create-skill** /
**create-task** / **create-persona** / **create-reference**, then write the real content
through the per-kind authoring skills (`ktw-compose:/skill/create-skill`,
`ktw-compose:/skill/create-task`, …). Rebuild after every embed change.

A **CWM pack** (`embed/cwm/`) is not scaffolded: add the folder to the
`//go:embed` line, and that is all — the kit's default pack reads `embed/cwm`
as a part of the binary's pack with no option in `cmd/bootstrap.go`
(`sidecarkit.WithoutPack()` drops it), every pack kind included (a pack folder
no kind reads refuses the boot). Its body schemas sit flat in
`embed/cwm/data-types/`. `app/ktw-demo` is the model. The pack's content is `ktw-compose:/skill/create-object-type`,
`ktw-compose:/skill/create-capability`, `ktw-compose:/skill/create-object-panel`;
its translations `ktw-compose:/skill/translate-contributions`.

## 6. Test

A files-only sidecar has no service to call — its surface is the packs.
Confirm they are discoverable:

```bash
./bin/<name> skills list              # the embed/skills/
./bin/<name> resources list           # skills + tasks + references + configs
```

For the full read-only verification battery (CLI shape, every surface, all
contributions reachable), load `ktw-compose:/skill/sidecar-testing`.

## Key patterns

- `appcore.BootstrapCLI` — the entire boot lifecycle: config loading, CLI
  generation, sidecar method-call serving, NATS connection, embed FS mounting.
- `sidecarkit.NewSidecar(AppConfig, opts...)` — the bootstrap builder; for a
  files-only sidecar it wires manifest + skills + tasks + references + the
  sidecar method-call surface from the `embed/` tree alone.
- The whole point: **all value is in `embed/`**. Reaching for a service, a
  `WithDomain`, or an `internal/extensions/<domain>` import means you have
  left files-only territory — route to the code-bearing track.

## Runtime directory layout

- **Workspace** (per-project): `<KTW_WORKSPACE>/.katyoti/`
  - `config/Univers.yaml` — sidecar list + universe identity
  - `config/<ServiceName>.yaml` — service overrides (override `embed/config/`)
  - `sidecars/<sidecar-name>/` — sidecar-specific config dir
- **Cross-workspace global root**: `~/.config/katyoti/`
  - `universes.yaml` — registry of known universes
- **Data dir** (sessions, logs): `<XDG_DATA>/katyoti/universes/<universe>/`

Embedded `embed/config/<ServiceName>.yaml` files are first-run templates only —
runtime reads from the workspace's `.katyoti/config/` after first boot.
