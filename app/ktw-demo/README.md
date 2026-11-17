# ktw-demo

Opt-in sidecar that ships demo / hello-world tasks for the ktw-agents
ecosystem, and the **reference extension**: the `signoff` capability, which
uses every mechanism a sidecar can give CWM. Universes that list `ktw-demo`
among their sidecars get the demos and the sign-off capability. Production
universes can leave it out to keep the catalog clean.

## The sign-off capability

Sign-off items (and any object taking the `signable` aspect) carry a sign-off:
reviewers, approvals, a verdict and notes. The pack is `embed/cwm/` (an object
type, two capabilities, an aspect, a workflow, a gate and a gate requirement,
a policy, an edge type, a hook type, a bootstrap hook, two panels, an agent
context, data-types), with `embed/tasks/signoff-request.md`,
`embed/tasks/signoff-summary.md`, `embed/skills/signoff-an-object/` and
`embed/guide/signoff-an-item.yaml`; every text a person reads has its French
translation (`.i18n.yaml`). The service behind it is
`internal/extensions/demosignoff`, on its slot `ktw-demo:signoff/<verb>`
(panels, acts, `signedoff()`, and the hooks CWM asks at its points). The guide that walks it:
[what a capability and a sidecar give](../../modules/cwm/objects/what-a-capability-and-a-sidecar-give.cwm.md) §7.
The scenario case `test_files/scenarios/demo-showcase` drives it end to end:

```sh
ktw-compose scenario install demo-showcase --root <root> --bin <bin>
ktw-compose scenario run demo-showcase --root <root>
```

## Composes modules

| Module | Role |
|---|---|
| [task](../../modules/tasks) (library only) | Task library publication |
| [framework](../../modules/framework) | Bootstrap |
| [catalog](../../modules/contributions) | Manifest |
| [scripts](../../modules/scripts) | (placeholder; no scripts shipped today) |
| [cwm](../../modules/cwm) | The sign-off pack (`embed/cwm`) and its service |

## Tasks shipped

| Task | Type | Purpose |
|---|---|---|
| `hello-world` | task | Simple greeting + file write — first-run sanity check |
| `document-demo` | task | Demonstrates the `create-document` tool (Markdown + HTML tabs) |
| `demo-delegate-magentic` | workflow | Magentic shared-ledger pattern (manager + 2 specialists) |
| `demo-delegate-pipeline` | workflow | Sequential agent pipeline (researcher → analyst → writer) |
| `demo-delegate-parallel-fallback` | workflow | Parallel branches with `on_error.fallback` (zero-LLM simulator) |
| `demo-delegate-tree` | workflow | Multi-level, multi-mode delegation tree (zero-LLM simulators) |
| `demo-delegate-team-tree` | workflow | Coordinator forks two teams; each lead (LLM) synthesizes 3 simulator workers (2 LLM costs) |
| `demo-delegate-team-coordination` | agent | Coordinator delegates to 3 specialist workers in parallel |
| `demo-delegate-team-messaging` | agent | Team-leader spawns 2 teammates exchanging mailbox messages |
| `demo-crud-book` | task | Schema-driven Book CRUD (agent-mediated, Mode A) |
| `demo-crud-book-declarative` | task | Book table with declarative host-handled row-click (Mode B) |

## How to opt in

Add `ktw-demo` to your universe's `extensions.yaml` (via the GUI's
Edit Universe → Sidecars tab, or by editing the file directly). Once
selected, the `ktw-agents` core spawned for that universe discovers
`ktw-demo` via `NeighborLookup` (scans `bin/`) and merges its tasks
into the universe's `CompositeTaskLibrary`.

Verify with:

```sh
ktw-agents tasks list   # demos appear when ktw-demo is selected
```

## Build

```sh
make ktw-demo           # builds bin/ktw-demo.exe
```

`ktw-demo` follows the standard ktw-agents sidecar pattern (same
scaffolding as the other sidecars). Beside its static content it publishes
the sign-off service.

## Adding more demo content

- **Tasks**: drop a new task directory into `embed/tasks/<name>/` with
  the standard `TASK.md` frontmatter. Rebuild — the embed loader picks
  it up automatically.
- **Skills**: drop a folder into `embed/skills/<name>/` with its `SKILL.md`.
