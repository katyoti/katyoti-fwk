# ktw-univers

CLI/sidecar for universe **catalog** operations in the ktw-agents ecosystem.

Provides 5 commands: `list`, `info`, `create`, `delete`, `scan-inventory`.
Each is one-shot — opens the registry, performs its operation, exits.

`ktw-univers` is **not an orchestrator**. It does not spawn ktw-agents
cores, it does not maintain an active-universes map, and it has no daemon
mode. Active-universe management lives in ktw-gui
([the ActiveCore model](../../modules/gui/concepts.md#2-the-activecore-model)); each active
universe runs its own `ktw-hub` ([the decision on which binary hosts the bus](../../modules/bus/decisions/OD-003-which-binary-hosts-the-bus.cwm.md)).

## What is a universe?

A universe is a typed scope: a configured set of sidecars + content-types +
per-universe configuration. Use case: a consultant switching between client
contexts (e.g., "Acme" → "Globex"), each with its own LeanIX
instance, Confluence space, etc. — one click, no manual reconfiguration.

Each universe has its own home folder containing `config/Univers.yaml` (the
manifest), per-universe configs, skills, tasks, and extensions. ktw-agents
runs with `KTW_HOME=<universe-home>` to act in that universe's context.

## Quick start

```bash
# Scan for available sidecars on the system
ktw-univers scan-inventory

# Create a new universe
ktw-univers create acme \
    --display "Acme" \
    --color violet --icon R

# List registered universes
ktw-univers list

# See full details
ktw-univers info acme

# Archive a universe (admin universe is protected)
ktw-univers delete acme
```

## Commands

| Command | Purpose |
|---|---|
| `list` | List all registered universes |
| `info <name>` | Full details for a named universe (Univers.yaml contents) |
| `create <name> [flags]` | Create a new universe folder + register in catalog |
| `delete <name>` | Archive a universe folder + remove from catalog |
| `scan-inventory` | Fresh scan of `$PATH` for `ktw-*` binaries |

All commands are inline (one-shot): the binary reads/writes the registry
file (`~/.config/ktw-agents/universes.yaml`) and exits. Activation is owned by
ktw-gui.

## Admin universe

`ktw-univers` auto-creates an `admin` universe on every boot at
`~/.config/ktw-universes/admin/`. The admin universe is **protected** —
`ktw-univers delete admin` errors. It serves as the universe-management
host (see [the decision on where a fresh install manages its universes](../../modules/univers/decisions/OD-001-where-does-a-fresh-install-manage-its-universes.cwm.md)).

## Sidecar surface

`ktw-univers` is a first-class member of the ktw-* sidecar contract. Beyond
the 5 catalog commands above, it exposes the standard sidecar surface:

```bash
ktw-univers config check      # verify dependencies (no required external deps)
ktw-univers llm info                                    # LLM usage guide + tools + skills
ktw-univers llm manifest                                # full self-describe (identity + tools + types)
ktw-univers sc skills list                              # discover skills (ships universe-management)
ktw-univers sc skills get-instructions ktw-univers:universe-management
```

The `universe-management` skill ([embed/skills/universe-management.md](embed/skills/universe-management.md))
is the LLM's primary entry point for universe CRUD. It documents all 5 KTSP
methods, decision trees, and example workflows.

## Filesystem layout

```
~/.config/ktw-agents/                         (cross-universe global root)
├── .env                              (default API keys + cross-universe env)
├── config/                           (cross-universe configs — GUI state, etc.)
├── sidecars.yaml                     (global sidecar discovery — populated by `ktw-univers scan-inventory`)
└── universes.yaml                    (registry: name → universe home path)

~/.config/ktw-universes/                      (default location for universes)
├── admin/                            (auto-created, protected)
│   ├── .env
│   ├── config/Univers.yaml           (manifest)
│   ├── config/Settings.yaml
│   └── extensions.yaml               (per-universe sidecar membership; admin includes ktw-univers)
├── acme/
└── globex/
```

### `sidecars.yaml` vs `extensions.yaml`

| File | Scope | Answers | Populated by |
|---|---|---|---|
| `~/.config/ktw-agents/sidecars.yaml` | global | "what is INSTALLED on this machine" | `ktw-univers scan-inventory` (write-through) |
| `<universe-home>/extensions.yaml` | per-universe | "which sidecars does THIS universe USE" | `ktw-univers create` writes initial; user edits later |

extensions.yaml is a strict subset of the sidecars.yaml catalog.

A universe folder can live anywhere on disk — pass `--home <path>` to
`ktw-univers create` to register a non-default location.

## See also

- Decisions: [the decision on where the cross-universe management services live](../../modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md) (the admin universe serves the management services), [the decision on which binary hosts the bus](../../modules/bus/decisions/OD-003-which-binary-hosts-the-bus.cwm.md) (one `ktw-hub` per universe)
- Active universes: [gui concepts §2](../../modules/gui/concepts.md#2-the-activecore-model)
- Layout: [katyoti layout](../../modules/katyoti/layout.md), [framework environment §2](../../modules/framework/environment.md#2-environment--workspace--3-derived-roots)
- ADR: [the decision on where a fresh install manages its universes](../../modules/univers/decisions/OD-001-where-does-a-fresh-install-manage-its-universes.cwm.md)
- Implementation package: [internal/extensions/orchestrator/](../../internal/extensions/orchestrator/)
