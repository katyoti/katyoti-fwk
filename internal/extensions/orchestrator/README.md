# orchestrator extension (ktw-univers sidecar package)

Implementation package for the `ktw-univers` CLI/sidecar. Despite the legacy
package name, this package no longer "orchestrates" anything — it provides
universe **catalog** operations (list, get, create, delete, scan-inventory).

Active-universe management (hold the active map, switch the displayed
universe) lives in ktw-gui ([gui concepts §2](../../../modules/gui/concepts.md#2-the-activecore-model)),
which runs one ktw-hub per active universe ([the decision on which binary hosts the bus](../../../modules/bus/decisions/OD-003-which-binary-hosts-the-bus.cwm.md)).

Extension package layout follows
[modules/framework/extension-layout.md](../../../modules/framework/extension-layout.md).

## Package layout

```
internal/extensions/orchestrator/
├── api/                              (public contract — no framework imports)
│   └── types.go                      (OrchestratorService interface + ServiceDef + I/O types)
├── local/                            (PublishService declaration)
│   └── services.go                   (Extension() — wires the service into PublishedServicesPoint)
├── commands/                         (Cobra commands for ktw-univers)
│   ├── util.go                       (DI helpers, JSON output, table formatters)
│   └── list.go, info.go, create.go, delete.go, scan_inventory.go
└── internal/core/                    (private implementation)
    ├── service.go                    (Service — implements OrchestratorService)
    ├── manifest.go                   (Univers.yaml load/save + Manifest type)
    ├── registry.go                   (universes.yaml load/save)
    ├── inventory.go                  (sidecars.yaml — global discovery catalog)
    └── extensions.go                 (extensions.yaml — per-universe membership)
```

## Architecture summary

Per [the decision on where the cross-universe management services live](../../../modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md):

- **One published service**: `OrchestratorService` with 5 KTSP methods —
  `ListUniverses`, `GetUniverse`, `CreateUniverse`, `DeleteUniverse`,
  `ScanInventory`.
- **`Service` singleton**: reads/writes the registry
  (`~/.config/ktw-agents/universes.yaml`), reads/writes per-universe manifests
  (`<home>/config/Univers.yaml`), and scans `$PATH` for ktw-* binaries.
- **No active-universes map**: ktw-univers is one-shot per CLI invocation.
  Long-lived state (which universes are active, where their cores are
  running) is owned by ktw-gui's Go-side supervisor.
- **No process spawning**: ktw-univers does not exec child ktw-agents
  processes. Activation lives in ktw-gui.

## Public API

```go
// Resolve via DI in any binary that imports orchestrator/local:
svc, err := application.UseService[api.OrchestratorService](ctx)

out, _ := svc.ListUniverses(ctx, api.ListUniversesInput{})
info, _ := svc.GetUniverse(ctx, api.GetUniverseInput{Name: "acme"})
new, _ := svc.CreateUniverse(ctx, api.CreateUniverseInput{Name: "acme", DisplayName: "Acme", HomePath: "/path/to/acme"})
del, _ := svc.DeleteUniverse(ctx, api.DeleteUniverseInput{Name: "acme"})
inv, _ := svc.ScanInventory(ctx, api.ScanInventoryInput{})
```

## Wire-up in ktw-univers

```go
// app/ktw-univers/cmd/bootstrap.go
return []application.Extension{
    appcore.NewAppExt(appcore.AppConfig{ AppName: "ktw-univers", ... }),
    orchestratorlocal.Extension(),                                   // publish OrchestratorService
    ktslocal.AutoKTSPContainer("ktw-univers", "...", version),       // build KTSP container
    appcore.CommandExt(commands.NewListCommand()),
    appcore.CommandExt(commands.NewInfoCommand()),
    appcore.CommandExt(commands.NewCreateCommand()),
    appcore.CommandExt(commands.NewDeleteCommand()),
    appcore.CommandExt(commands.NewScanInventoryCommand()),
}
```

Auto-CLI generation is suppressed for the `OrchestratorService` slot via
`WithKTSPSlotTags({"cli": "false"})`; the commands above provide nicer UX
for human users.

## Admin universe (auto-created)

On every `NewService` boot, the service ensures the admin universe exists
(per [the decision on where a fresh install manages its universes](../../../modules/univers/decisions/OD-001-where-does-a-fresh-install-manage-its-universes.cwm.md)):

- **Path**: `~/.config/ktw-universes/admin/` (cross-platform via `os.UserHomeDir`)
- **Auto-creation**: idempotent + self-healing — if either the folder OR the
  registry entry is missing, the absent half is recreated. Both present →
  no-op.
- **Protected**: registry entry has `Protected: true`; `DeleteUniverse("admin")`
  errors with a clear message.
- **Default sidecar membership**: empty in v1 (`ktw-univers-sidecar-surface`
  adds ktw-univers itself to admin's `extensions.yaml` later in ms06c).

Test isolation: production code calls `adminUniverseHome()` (uses the real
`os.UserHomeDir()`). Unit tests pass a TempDir path directly to
`ensureAdminUniverse(registry, homePath)` to avoid touching the user's real
home directory.

## sidecars.yaml + extensions.yaml split

Two yaml files manage the sidecar lifecycle (added by
`ktw-univers-sidecar-surface`):

- **`~/.config/ktw-agents/sidecars.yaml`** (global discovery): catalog of sidecars
  installed on the system. Populated by `ScanInventory` (write-through merge
  with `LastSeen` timestamps). Entries absent from a fresh scan are preserved
  (the binary may simply be off-PATH temporarily).
- **`<universe-home>/extensions.yaml`** (per-universe membership): the subset
  of installed sidecars that this universe USES. Written initially by
  `CreateUniverse`; edited by the user later. Read by ktw-agents core at
  boot time to know which sidecars to register.

Admin universe's `extensions.yaml` ships with `ktw-univers` as its single
member (auto-populated when `writeAdminFolderSkeleton` runs on first boot).

## Tests

```bash
go test ./internal/extensions/orchestrator/...
```

Unit tests cover: registry CRUD, manifest round-trip + numeric version field,
service CRUD operations, name validation, admin auto-creation +
self-healing + delete protection, sidecars.yaml load/save + merge semantics,
extensions.yaml round-trip.

## See also

- Decisions: [the decision on where the cross-universe management services live](../../../modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md), [the decision on which binary hosts the bus](../../../modules/bus/decisions/OD-003-which-binary-hosts-the-bus.cwm.md)
- Layout: [katyoti layout](../../../modules/katyoti/layout.md)
- ADR: [the decision on where a fresh install manages its universes](../../../modules/univers/decisions/OD-001-where-does-a-fresh-install-manage-its-universes.cwm.md)
- Layout: [framework/extension-layout.md](../../../modules/framework/extension-layout.md)
- Pattern: [authoring a service](../../../modules/framework/authoring-a-service.md#5-publication--local)
- Sibling binary: [app/ktw-univers/](../../../app/ktw-univers/)
