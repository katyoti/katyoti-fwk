---
name: universe-management
description: >-
  Manage universes — named contexts of sidecars + per-universe configuration —
  via the ktw-univers catalog service. Use when the user wants to list,
  inspect, create, delete, or migrate universes, or when they ask "what
  universes exist", "set up a new client universe", or anything about
  switching between contexts. Covers all 5 KTSP methods (ListUniverses,
  GetUniverse, CreateUniverse, DeleteUniverse, ScanInventory) plus the
  workflows that combine them.
requires:
  - orchestrator
---

# Universe Management

A **universe** is a named context that bundles together:

- A **home folder** (e.g., `~/.config/ktw-universes/acme/`) holding the universe's `Univers.yaml` manifest, per-universe config, skills, tasks, and `extensions.yaml` (sidecar membership)
- A **set of sidecars** the universe uses (subset of the globally-installed sidecars catalog)
- A **scope** — when ktw-agents runs with `KTW_HOME=<universe-home>`, all skills, tasks, configs, and sidecars are resolved against that universe's layered home overlay

Typical use: a consultant switches between client contexts ("Acme" → "Globex") with one click; each universe has its own LeanIX instance, Confluence workspace, document templates, etc.

This skill is the LLM's primary entry point for universe CRUD. The full surface lives in the `orchestrator` KTSP service exposed by the `ktw-univers` sidecar.

## Architecture context

- Catalog file: `~/.config/ktw-agents/universes.yaml` (registry — name → home path)
- Universe manifests: `<home>/config/Univers.yaml`
- Global sidecars catalog: `~/.config/ktw-agents/sidecars.yaml` (populated by `scan-inventory`)
- Per-universe sidecar membership: `<home>/extensions.yaml`
- Admin universe: auto-created at `~/.config/ktw-universes/admin/`, `protected: true`, ships this skill

`ktw-univers` does NOT spawn ktw-agents cores or hold an active-universes map — that lives in ktw-gui. ktw-univers is a one-shot catalog operator.

## Method reference

| Method | Purpose | Key inputs | Key outputs |
|---|---|---|---|
| `ListUniverses` | List the catalog | (none) | `Universes[]` — name, displayName, description, icon, color, homePath, protected |
| `GetUniverse` | Full info for one universe | `name` | `Summary`, `Sidecars[]` (membership), `LayeredHomes[]` |
| `CreateUniverse` | Create a new universe | `name`, `displayName`, `description`, `icon`, `color`, `homePath`, `sidecars[]` | full `UniverseInfo` of the created universe |
| `DeleteUniverse` | Archive a universe + remove from registry | `name` | `Archived` (bool), `ArchivePath` |
| `ScanInventory` | Probe `$PATH` for `ktw-*` binaries | (none) | `Sidecars[]` — name, binaryPath, version, source |

All methods return errors on:
- Invalid input (e.g., bad universe name slug)
- Conflicts (e.g., universe name already exists)
- Protected universes (e.g., `DeleteUniverse("admin")`)

### Naming rules

Universe names must match `^[a-z0-9-]{1,64}$` — lowercase letters, digits, hyphens; no leading/trailing hyphen; max 64 chars. The slug `admin` is reserved for the auto-created admin universe.

### Color preset

`color` accepts one of: `violet`, `blue`, `teal`, `lime`, `yellow`, `orange`, `red`, `pink`, `grape` (Mantine color tokens). Used for the universe's icon avatar in the GUI.

## Decision tree

**User asks "what universes exist?"** → `ListUniverses`

**User asks for details about a specific universe** → `GetUniverse(name=…)` to get the full manifest including sidecar membership

**User wants to create a new universe**:
1. Optional: `ScanInventory` to learn what sidecars are installed on the system
2. Ask the user for the slug, display name, description, icon, color, and which sidecars to include
3. `CreateUniverse(...)` with the chosen sidecars
4. Tell the user the universe was created and how to switch to it (in ktw-gui: click the new icon in the IconRibbon)

**User wants to delete a universe**:
1. Confirm the user really wants to (deletion archives the folder under `<parent>/.archived/<name>/` but the registry entry is removed)
2. `DeleteUniverse(name=…)`
3. If the universe is protected (admin), report the error verbatim — do NOT try to bypass

**User wants to "clone" a universe**:
- There is no Clone primitive in v1. Use `CreateUniverse` with the same sidecar list, then point the user at the source universe's config files for manual copy of any per-universe overrides they care about.

## Example workflow — create a new client universe

User: "Set up a new universe for client Acme. We use ktw-kb and ktw-docs for them."

1. `ScanInventory` — confirm both sidecars are installed:
   ```json
   {
     "sidecars": [
       {"name": "ktw-kb",     "binaryPath": "/usr/local/bin/ktw-kb",     "version": "1.0.0", "source": "PATH"},
       {"name": "ktw-docs",   "binaryPath": "/usr/local/bin/ktw-docs",   "version": "1.0.0", "source": "PATH"}
     ]
   }
   ```

2. `CreateUniverse`:
   ```json
   {
     "name": "acme",
     "displayName": "Acme",
     "description": "Primary client — strategy & architecture practice",
     "icon": "R",
     "color": "violet",
     "homePath": "/home/user/ktw-universes/acme",
     "sidecars": [
       {"name": "ktw-kb",     "binaryPath": "/usr/local/bin/ktw-kb"},
       {"name": "ktw-docs",   "binaryPath": "/usr/local/bin/ktw-docs"}
     ]
   }
   ```

3. Confirm to the user:
   > Created universe **Acme** at `/home/user/ktw-universes/acme/`. It includes ktw-kb and ktw-docs. To activate it, click the **A** icon in the IconRibbon, or run `ktw-univers info acme` to inspect the manifest.

If `homePath` is omitted, the default is `~/.config/ktw-universes/<name>/`.

## Example workflow — list active universes

User: "What universes do I have?"

1. `ListUniverses`
2. Format the response as a table or short list:
   > You have 3 universes:
   > - **admin** (protected, A in gray) — Universe management
   > - **acme** (A in violet) — Primary client
   > - **globex** (G in blue) — Secondary client

If the user asks "which one am I in?" — that's a GUI/runtime concern (the `displayedUniverseName` lives in localStorage per ktw-gui window). This skill cannot answer it; suggest the user check the highlighted icon in the IconRibbon.

## Example workflow — inspect a universe's sidecars

User: "What sidecars does the Globex universe use?"

1. `GetUniverse(name="globex")`
2. Walk the `Sidecars[]` array; for each, report the `name` and `transport`:
   > Globex uses 2 sidecars:
   > - **ktw-kb** (stdio) — config dir: sidecars/ktw-kb
   > - **ktw-docs** (stdio) — config dir: sidecars/ktw-docs

If `LayeredHomes` is non-empty, mention them — they're additional skill/task overlay paths.

## Error handling

When a method returns an error, surface it verbatim. Do NOT retry on:
- Validation errors (`invalid universe name`, `display_name is required`)
- Conflicts (`already exists in registry`, `home path already exists`)
- Protected universes (`universe "admin" is protected`)

For transient errors (filesystem permissions, disk full), report the error and suggest the user check filesystem state.

## Filesystem side effects

`CreateUniverse` writes:
- `<homePath>/config/Univers.yaml` — the manifest
- `<homePath>/.env` — empty placeholder for per-universe env vars
- `<homePath>/config/Settings.yaml` — empty placeholder
- An entry in `~/.config/ktw-agents/universes.yaml`

`DeleteUniverse` moves `<homePath>` to `<parent>/.archived/<name>/` (data is preserved for recovery, just out of the registry). It does NOT delete the data permanently.

`ScanInventory` is read-only against the system but writes the discovered inventory to `~/.config/ktw-agents/sidecars.yaml` for fast subsequent reads.

## What this skill does NOT do

- **Activate / display / deactivate** universes — that's ktw-gui's responsibility (the active-universes map lives there)
- **Switch the displayed universe** — happens in the ktw-gui IconRibbon click handler, not via ktw-univers
- **Configure sidecar internals** — per-sidecar settings (LeanIX URL, Confluence space) live in `<universe-home>/config/sidecars/<name>/Settings.yaml` and are edited by hand or by sidecar-specific skills
- **Spawn child processes** — ktw-agents core (per universe) and ktw-gui handle process lifecycle

If the user asks for any of the above, say so and point them at the right tool (ktw-gui for activation, the sidecar's own skill for its config).
