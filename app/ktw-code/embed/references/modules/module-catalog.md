# Module Catalog

Inventory of candidate modules in this codebase, with their primary couplings and a recommended roadmap for the rock-solid program.

This catalog is **a starting point**, not authoritative. Every entry needs validation during its own bootstrap (does the code really live where this says? are the couplings what this claims?). Treat it as a seed for Phase B+ planning.

## Definition of "module" (for this method)

A **module** is a coherent subtree of source code that:

- Has a clear public surface that other modules consume (exports, interfaces).
- Has its own concerns (one module ≠ everything in `internal/extensions/`).
- Is small enough to author a charter for (≤~30k LOC; otherwise consider splitting).
- Big enough to deserve a charter (a single 50-LOC helper isn't a module).

For ktw-agents, this maps to:
- Most `internal/extensions/<name>/` subtrees.
- Some `internal/<name>/` subtrees (`appcore`, `core`, `kts`).
- Each `app/<binary>/` (sidecar binaries).

A module may span multiple folders (e.g. the `agent` module may include parts of `internal/extensions/agent/` AND parts of `internal/extensions/tools/`). Resolve this at bootstrap time per module.

## Cluster overview

The codebase has ~30 module candidates. Grouped into **6 logical clusters**:

| Cluster | What it owns | Modules |
|---|---|---|
| **A. Runtime core** | The agent + task + workflow + orchestration runtime | `tasks`, `agent`, `orchestration` |
| **B. Tool surface** | LLM-facing tools and their shared infrastructure | `tools/api`, `tools/file_*`, `tools/glob`, `tools/grep`, `tools/bash`, `tools/web_*`, `tools/ask_user`, `tools/wikipedia`, `tools/rest_api`, `tools/create_document`, `tools/display_ui_component`, `tools/complete_ui_component`, `workspace`, `mcp` |
| **C. Task strategies** | Alternative `Machine` implementations beyond agent-loop | `humangate`, `program`, `simulator`, `template`, `ledger` |
| **D. Skill ecosystem** | Skills, scripts, runners | `skills`, `scripts`, `runner` |
| **E. Plumbing / bridges** | Cross-binary transport, UI bridges, terminal | `kts` (transport), `displaybridge`, `terminal`, `web`, `appcore` |
| **F. Domain extensions** | Project-specific or experimental | `docs`, `leanix`, `code`, `notify`, `browse`, `kb`, `sync`, `echo`, `worktree`, `testharness`, `orchestrator`, `tui`, `schema` |

Cluster A is **load-bearing** for everything else. Bringing it to tier 1 first is the priority.

## Detailed inventory

### Cluster A — Runtime core (target: rock-solid first)

| Module | Source | Public surface | Tight couplings | LOC est. |
|---|---|---|---|---|
| **tasks** | `internal/extensions/task/` + `internal/extensions/simulator/` | `task/api/`, `task/api/engine/`, `task/api/engine/presets/`, `simulator/MachineStrategyDef` | depends on: `appcore`, `core/services`, `core/em`. Depended on by: `agent`, `orchestration`, all task strategies (cluster C), all task-emitting domain extensions. **Includes simulator** because (a) tightly coupled with `task/api`, (b) it's the canonical token-free L2/L3 test driver. | ~11.3k |
| **agent** | `internal/extensions/agent/` | `api/` (PluginHooks, AgentRunRequest, plugin contracts) | depends on: `tasks` (the engine), `tools/api`, `core/services`, LLM provider adapters. Depended on by: most domain extensions, `orchestration` | ~?k |
| **orchestration** | `internal/extensions/orchestration/` | exports plugin (delegate, spawn-teammate, send-message, check-messages tools) | depends on: `tasks` (Channel, EventBus, Spawner), `agent` (PluginHooks). Depended on by: `ktw-agents` bootstrap | ~?k |

These three are inseparable in practice. The `tasks` audit (2026-05-06) already covered some orchestration concerns (mailbox routing, cross-team isolation). A reasonable Phase B/C scoping:

- Phase B → `tasks` charter (covers the engine + some orchestration concerns)
- Phase C → `agent` charter (covers the agent runtime + plugin contracts)
- Phase D → `orchestration` charter (covers cross-agent comms, possibly merge with `agent` if scope allows)

### Cluster B — Tool surface

| Module | Source | Notes |
|---|---|---|
| **tools/api** | `internal/extensions/tools/api/` | Shared interface for all tools. Imported by 46 places. Critical to get right; small surface. |
| **tools/file_read**, **file-write**, **file-edit**, **glob**, **grep** | `internal/extensions/tools/<name>/` | Workspace file operations. Small, individually simple, security-sensitive collectively. |
| **tools/bash** | `internal/extensions/tools/bash/` | Shell execution. **Highest single-tool security risk.** The path-restriction work in `tasks` (T2 critical finding) is mostly about tool coverage gaps that touch bash. |
| **tools/web_fetch**, **web-search**, **wikipedia**, **rest_api** | `internal/extensions/tools/<name>/` | External I/O tools. SSRF / abuse concerns. |
| **tools/ask_user** | `internal/extensions/tools/ask_user/` | User-interaction surface. Already wired with HumanGate. |
| **tools/create_document**, **display_ui_component**, **complete_ui_component** | `internal/extensions/tools/<name>/` | UI-side tools. Coupled to `displaybridge` and the GUI runtime. |
| **workspace** | `internal/extensions/workspace/` | Aggregate plugin combining file tools + glob. Defines workspace semantics. |
| **mcp** | `internal/extensions/mcp/` | MCP bridge. Brings external (potentially-uncurated) tools into the agent loop. **Permission/threat surface.** |

**Cluster B-meta charter consideration**: rather than 15+ individual charters (one per tool), it may be cleaner to charter **one "tool surface" module** that covers tools/api + the security gates + a representative set of tools. Individual tools then get their own UCs within that one charter. Worth a Phase D decision.

### Cluster C — Task strategies

Each is a `Machine` implementation contributing to `MachineDomain`. Smaller surface, lower stakes individually.

| Module | Source | Purpose |
|---|---|---|
| ~~**callmethod**~~ | ~~`internal/extensions/callmethod/`~~ | **Removed 2026-06-10** — replaced by the workflow `kind: call` primitive (one synchronous ActionService invocation, no child machine). |
| **humangate** | `internal/extensions/humangate/` | Pauses for explicit human approval. |
| ~~**program**~~ | ~~`internal/extensions/program/`~~ | **Removed 2026-06-20** — superseded by the `bash` agent tool. Sidecar tool dependencies are declared via `WithTools` and surface as `Category="program"` in `sc config check`. |
| ~~**simulator**~~ | ~~`internal/extensions/simulator/`~~ | **Moved to Cluster A** — included in the `tasks` module (token-free Machine for L2/L3 testing; tightly coupled with `task/api`). |
| **template** | `internal/extensions/template/` | Renders a template. |
| **ledger** | `internal/extensions/ledger/` | Magentic shared-state ledger. |

These don't need individual rock-solid charters in the near term. A "task strategies" group charter or one-pager-each at the end of Phase D suffices.

### Cluster D — Skill ecosystem

| Module | Source | Public surface | Notes |
|---|---|---|---|
| **skills** | `internal/extensions/skills/` | `SkillService` interface, AgentPluginExtension | Progressive discovery (L0 title → L1 instructions → L2+ refs/scripts). Skill loaders are an EP. |
| **scripts** | `internal/extensions/scripts/` | Script discovery + execution | Coupled to skills; agent plugin exposes `scripts-run`. |
| **runner** | `internal/extensions/runner/` | Skill script runner | Wraps script execution with permission/lifecycle. |

Skill-script execution is **a code-execution surface** the LLM can drive — security-critical. Treat it with `tools/bash`-tier seriousness. The `skill-script-sandboxing-review` task from the original ms07d proposal lives here.

### Cluster E — Plumbing

| Module | Source | Notes |
|---|---|---|
| **kts** | `internal/kts/` | KTSP transport (WebSocket, multi-container). Cross-binary infrastructure. |
| **appcore** | `internal/appcore/` | Sidecar bootstrap, config, common entry points. |
| **core** | `internal/core/` | application, services, em, config, strategies, loader, reflect, union, publish. THE foundational layer. |
| **displaybridge** | `internal/extensions/displaybridge/` | Bridge to ktw-display TS sidecar. |
| **terminal** | `internal/extensions/terminal/` | Terminal protocol implementation. |
| **web** | `internal/extensions/web/` | Web extension (browser interactions / web UI). |

Plumbing modules are foundational; charters here drift slowly. Bring up after cluster A but before cluster B.

### Cluster F — Domain extensions

Smaller, more focused, less load-bearing. Charter them individually only if/when they reach production stakes.

| Module | Source | Status |
|---|---|---|
| docs | `internal/extensions/docs/` | doc-discovery + tools |
| leanix | `internal/extensions/leanix/` | LeanIX integration |
| code | `internal/extensions/code/` | code-analysis sidecar |
| notify | `internal/extensions/notify/` | notification backend |
| browse | `internal/extensions/browse/` | browsing extension |
| kb | `internal/extensions/kb/` | knowledge base |
| sync | `internal/extensions/sync/` | sync extension |
| worktree | `internal/extensions/worktree/` | git worktree management — **may merge into tasks** for the spawn-isolation work |
| testharness | `internal/extensions/testharness/` | test infrastructure |
| orchestrator | `internal/extensions/orchestrator/` | (distinct from `orchestration` — investigate) |
| tui | `internal/extensions/tui/` | TUI |
| schema | `internal/extensions/schema/` | data DSL parser+normalizer |

### Apps (binaries)

| App | Notes |
|---|---|
| ktw-agents | The main runtime. Composes everything in cluster A+B+C+D+E. Charter here is more "integration UCs" than module-level. |
| ktw-utils | Session/journal inspection. Used by L3 tests; charter at tier-2 for `tasks`. |
| ktw-code | Code analysis sidecar. **Hosts the `code-module` skill itself.** |
| ktw-docs, ktw-notify, ktw-display, ktw-kb, ktw-sync, ktw-univers | Domain sidecars; charter lazily. |
| ktw-gui | Wails desktop GUI. Larger frontend story; separate quality program likely. |
| ktw-test | Test harness binary. **L2 test infra; charter is "does it correctly drive subjects?"** |
| ktw-demo | Demo binary; no charter expected. |

## Coupling map (high-level)

```
                    ┌──────────────┐
                    │  appcore +   │   (core layer; everything depends on it)
                    │  core + kts  │
                    └──────┬───────┘
                           │
                  ┌────────▼─────────┐
                  │      tasks       │   ◀── 11k LOC; 73 files; the engine
                  └────────┬─────────┘
                           │
                  ┌────────▼─────────┐                ┌────────────────┐
                  │      agent       │ ───────────▶   │   tools/api    │
                  └─┬───────┬────────┘                └────────┬───────┘
                    │       │                                  │
       ┌────────────▼───┐ ┌─▼──────────────┐    ┌──────────────▼──────────┐
       │ orchestration  │ │   skills /     │    │   tools/file_*, bash,   │
       │  (mailbox,     │ │   scripts /    │    │   web_*, ask-user, ...  │
       │   delegate)    │ │   runner       │    └─────────────────────────┘
       └────────┬───────┘ └────────────────┘                  ▲
                │                                              │
                └──────────────────────────────────────────────┘
                          (spawn-teammate / delegate
                           goes through tools, calls back into tasks)

Task strategies (cluster C: humangate, program, simulator,
template, ledger) all depend on tasks (engine).

Domain extensions (cluster F) are leaves; they depend on agent or tools.

Sidecar binaries depend on everything they bundle.
```

The strongly-connected core is **`tasks ↔ agent ↔ orchestration ↔ tools/api`**. Until those four are stable, charters for the rest are sandcastles.

## Recommended rock-solid roadmap

| Phase | Module | Tier target | Rationale |
|---|---|---|---|
| **B** | tasks | 1 | Audit done; the engine; load-bearing for everything |
| **C** | agent | 1 | Plugin contracts must be stable before cluster B can rely on them |
| **D** | orchestration | 1 | Cross-agent comms is the most prompt-injection-prone surface |
| **E** | tools/api + workspace + bash | 1 (grouped charter) | Security-sensitive set; sane to charter together |
| **F** | skills + scripts + runner | 1 (grouped) | Skill-script execution is a major attack surface |
| **G** | kts (transport) | 1 | Cross-binary reliability |
| **later** | individual tools, domain extensions, sidecars | 1-2 as needed | One per binary; each is small |

This is a **multi-quarter roadmap**, not a sprint. The L3 skill (when it exists) automates each phase; the human work is per-charter UC authoring + audit triage.

## Module boundary — open questions

For the `agent` module charter (Phase C), resolve these at bootstrap:

1. **Does `agent` include `internal/extensions/tools/`?** Probably not — different concerns, different change cadence. Tools get their own grouped charter (cluster B).
2. **Does `agent` include `internal/extensions/orchestration/`?** Probably yes — orchestration is implemented as agent plugins (delegate, spawn-teammate, mailbox tools). Hard to author a coherent agent charter without including these.
3. **Does `tasks` include `internal/extensions/worktree/`?** The fork-mode spawn behavior is tightly coupled to git worktrees. Possibly yes, possibly merged into tasks's `internal/taskplugin/` already. Verify at bootstrap.

These boundaries are decisions, not facts. The charter records the decision; the catalog just flags them.

## Maintenance

This catalog is a snapshot at 2026-05-06. Drift sources:

- New extensions added → add a row.
- Modules merged or split → update couplings.
- Charters created → link the module's row to its charter (`/modules/<name>/charter.md`).
- Phase completion → update the roadmap table.

A future task: tooling that auto-generates this catalog from `internal/extensions/*` and import analysis. Out of scope for Phase A.
