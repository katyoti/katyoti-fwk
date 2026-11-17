# References — code-module skill

Reference material loaded on demand by the [`code-module`](../../skills/code-module.md) skill. The skill itself is a lean orchestrator; depth lives here. Progressive discovery: load only what's relevant to the current verb.

## How to load

```bash
ktw-code llm resources read ktw-code:/reference/modules/<file>.md
ktw-code llm resources read ktw-code:/reference/modules/<subdir>/<file>.md
```

## Layout

```
app/ktw-code/embed/references/modules/
├── README.md                                  ← you are here
│
├── ── Method docs (universal, per-method) ──
├── charter-template.md                        Per-module charter format
├── workflow.md                                Lifecycle loop (Bootstrap → Specify → Plan → Execute → Verify → Audit → Promote → Retrospective)
├── test-layers.md                             L1/L2/L3 strategy (what + why per layer)
├── e2e-test-authoring.md                      .tasktest schema, assertion DSL, worked example, fixture conventions (HOW to write a test)
├── deliverables.md                            Bundle inventory + versioning per module
├── factory-rollup.md                          Portfolio view design
├── standards-enforcement.md                   Rule format + lint integration
├── alignment.md                               Charter-to-code drift detection (goals → concepts → specs)
├── quality-tooling.md                         Go-native tooling (golangci-lint + go test) vs SonarQube; on-request philosophy
├── module-catalog.md                          Inventory of candidate modules in ktw-agents
├── relationship-with-planification.md         Layering over planification (requests, to-dos, milestones)
├── lessons-from-tasks-bootstrap.md            Lessons #1 + #2 from the first real run
│
├── ── Per-verb references ──
└── verbs/
    ├── README.md                              Index of the 8 verbs
    ├── charter.md
    ├── status.md
    ├── lint.md
    ├── plan-tier.md
    ├── verify.md
    ├── audit.md
    ├── promote.md
    └── factory-status.md
│
├── ── Templates ──
└── templates/
    ├── README.md
    └── template.tasktest                      Master L2 scenario template (copied per-module by charter verb)
```

## Recommended reading order

For a first read of the method:

1. [`relationship-with-planification.md`](relationship-with-planification.md) — the layering frame; sets the mental model
2. [`charter-template.md`](charter-template.md) — the central per-module artifact
3. [`workflow.md`](workflow.md) — how the charter is exercised through the lifecycle
4. [`test-layers.md`](test-layers.md) — L1/L2/L3 strategy
5. [`e2e-test-authoring.md`](e2e-test-authoring.md) — how to write a test from a DoD
6. [`deliverables.md`](deliverables.md) — what each module ships at promotion
7. [`module-catalog.md`](module-catalog.md) — the candidate inventory
8. [`lessons-from-tasks-bootstrap.md`](lessons-from-tasks-bootstrap.md) — first-run retrospective with method-evolution lessons

Per-verb references in [`verbs/`](verbs/) are loaded on demand by the skill when a user invokes a verb; usually no need to pre-read.

## Consistency rules for these references

- Each method doc has a self-contained scope; cross-references are explicit (relative paths to siblings)
- No method doc embeds module-specific content (e.g. doesn't cite `/modules/tasks/`); use abstract paths like `/modules/<module>/`
- Examples are concrete (real UC IDs, real audit findings) where they help; abstract where they don't
- The skill ([`/ktw-code/skills/code-module.md`](../../skills/code-module.md)) does NOT duplicate this content; it points here

## Status

**Phase A.5** — references migrated from `/modules/docs/` (project-level Phase A staging) into the embed tree on 2026-05-06. The skill is now lean (orchestrator + verb list + rules) and details load on demand.

`/modules/<name>/` (per-module bundles) stays at the project level — that's the runtime artifact location.
