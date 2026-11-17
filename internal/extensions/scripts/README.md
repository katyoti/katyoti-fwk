# scripts extension — DORMANT (no live consumer)

**Status: wired but disconnected.** This extension (the central `ScriptService`)
is kept as-is, but nothing actually consumes it yet. It is **not done/tested in
anger** — it is waiting for a real consumer to exercise and validate it. Treat
it as a design sketch that compiles, not a finished feature.

## Two different "scripts" — don't confuse them

| Package | Role | In use? |
|---|---|---|
| `internal/core/scripts` | **Local runner** — executes bun/python/bash bytes (`Run(RunInput{Name, Data, …})`). | **Yes — actively.** The one piece that is genuinely local: execution must happen where the interpreter + companion files live. |
| `internal/extensions/scripts` (this package) | **Central ScriptService** — discovers `embed/scripts/*`, publishes a `scripts` KTSP slot with per-script methods (`<provider>:scripts/<name>`), and the agent `scripts-run` / `scripts-list` tools. | **No.** Zero concrete callers. |

The real script executions in the repo both go through the **local runner
directly**, bypassing this service:

- ktw-docs's `elk-layout` → `DocService.LayoutDiagram` → `archdiagram.LayoutELKGraph` → `core/scripts.Run`.
- ktw-agents's `build-catalog.ts` → `skills-run-script` (the **skill** runner) → `core/scripts`.

## Why it's disconnected

- The only real script that ever lived behind this service (ktw-docs `elk-layout`)
  was a duplicate; it now has a typed home as `DocService.LayoutDiagram`. ktw-docs
  dropped `WithScripts()`.
- `sidecarkit` defaults scripts **off** (opt-in via `WithScripts()`), so no kit
  sidecar publishes an (empty) `ScriptService`.
- Only ktw-agents still wires it (`scriptslocal.Extension()` + `AgentPluginExtension()`
  + a loader for `build-catalog.ts`) — but build-catalog is invoked via the skill
  runner, so even there nothing calls a `<provider>:scripts/<name>` method.

**Recommended default:** to expose script-backed functionality, wrap it in a
**typed service method** (the `DocService.LayoutDiagram` pattern) over
`core/scripts`. Reach for this extension only when a script genuinely needs
**cross-binary or agent-driven** invocation that a typed method can't cover.

## The good idea, kept on purpose

`local/slot_methods.go` is worth keeping: auto-surfacing every discovered script
as a KTSP slot method `<provider>:scripts/<name>` gives a uniform callable
surface across services *and* scripts, and can simplify call sites. The mechanism
is sound — it just has no caller yet.

## Target shape (build only when a real consumer appears)

When a genuine cross-binary script consumer shows up, finish this as a first-class
**Contribution type**, mirroring how `tasks` work (a script is, like a task, a
runnable Contribution with an input schema, dispatched via `ActionService`):

1. **Script type Contribution** — `Type: "script"`, meta = name, usage,
   input/output schema, engine, companion files. (Replaces the bespoke
   `ScriptLoadersPoint` + `scripts.yaml`.)
2. **Catalogue** — discovery for free via `CatalogService` (List/Get), like skills.
3. **Central ScriptService** — thin dispatcher: `Describe(uri)` (schema → launch
   form) + `Run(uri, input)` routes to the hosting binary.
4. **Local runner** — `internal/core/scripts`, unchanged. The host's slot method
   receives the call and runs the bytes.

Until then: keep as-is, disconnected. Don't pay for the machinery before there's
a caller.
