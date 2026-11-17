---
name: test-artifact
description: "Behavioral-test method dispatcher for any katyoti artifact; load to pick and route to the cheapest way to test, verify, smoke, or debug a skill, task, agent, workflow, sidecar, or GUI by running it."
advertised: true
hashtags: [testing, verification, debug, method]
category: guide
---

# Testing a katyoti artifact

Every artifact is testable; the craft is picking the cheapest method that
answers the question. Identify the artifact and the question, pick from the
catalog, then load the method's skill and follow it.

**Tests and scripts run in bash** — Git Bash on Windows, never cmd.exe or PowerShell:
a check, a probe, a test script, a grade or a scenario step is a bash
command line. Windows can provide bash, Linux cannot provide PowerShell, so bash is the one
environment everywhere; PowerShell and cmd.exe also mangle quoted arguments (`-run 'A|B'`,
JSON).

## 1. The method catalog

| Method | Skill | Use case | Pros | Cons |
|---|---|---|---|---|
| **CLI probe** | `ktw-compose:/skill/test-cli-probe` | sidecar surfaces: tools/resources/skills load, contracts hold | zero LLM cost, deterministic, scriptable | tests the surface, not behavior |
| **stdio method-call** | `ktw-compose:/skill/test-ktsp-stdio` | the wire protocol itself: JSON-RPC framing, slot routing | exact wire-level truth | verbose, low-level |
| **Run + trace** | `ktw-compose:/skill/test-run-trace` | real agent/task behavior end-to-end with full evidence | richest evidence (every turn, tool call, token) | LLM cost, stochastic (n=1 is an anecdote) |
| **Dry-run** | `ktw-compose:/skill/test-dry-run` | workflow/task topology + event flow | zero LLM cost, deterministic, fast | simulated children — no real outputs |
| **Virtual terminal** | `ktw-compose:/skill/test-virtual-terminal` | interactive flows (ask-user), stdin injection, long runs | drives stdin/stdout over HTTP, scriptable interactivity | extra server moving part |
| **Terminal protocol** | `ktw-compose:/skill/test-terminal-protocol` | the NDJSON terminal protocol a GUI/TUI consumes | tests the exact client contract | protocol-level only |
| **Session journals** | `ktw-compose:/skill/test-journals` | post-run inspection and scoring of any past run | free (logs already exist), cross-journal correlation | after-the-fact only |
| **Hung run (pprof)** | `ktw-compose:/skill/test-hung-run` | a run that STALLS — no output, journals stop mid-loop, no visible prompt | reads the live goroutine stacks → exact blocking frame | needs `KTW_PPROF` set before launch; capture while still hung |
| **GUI via browser** | `ktw-compose:/skill/test-gui` | ktw-gui screens, panels, click-paths | tests what the user actually sees | dev-mode caveats, slowest loop |
| **Go + tasktest layers** | `ktw-compose:/skill/test-go` | software changes: units (L1), scenarios (L2), self-hosted (L3) | CI-grade, regression-pinned | code-level; doesn't see prompts/UX |
| **Scenario case** | `ktw-compose:/skill/test-scenario-case` | a whole flow on a real project, in its own universe, across sidecars, over hours — no person present | checkpointed stages (a failure replays alone), simulated user, checks a person would see | LLM cost; slowest to write |

Flows that sit ABOVE these methods and consume them:
- score a skill → `ktw-compose:/skill/skill-evaluation`
- iterate with user-validated changes → `ktw-compose:/skill/improve-artifact`
- full sidecar contract battery → `ktw-compose:/skill/sidecar-testing`
- audit a WHOLE existing sidecar (usage road → per-artifact quality → use-case
  reconciliation vs docs → proposals) → `ktw-compose:/skill/audit-sidecar`

## 2. Artifact → method matrix

| Artifact | First method | Also |
|---|---|---|
| **Skill** | CLI probe (parses, lists, loads) | run+trace (does an agent use it), skill-evaluation (score it) |
| **Task** | dry-run (topology) | run+trace (real behavior), virtual terminal (if interactive) |
| **Agent** | run+trace | journals (post-run), improve-artifact (iterate) |
| **Workflow** | dry-run + dry-trace | run+trace for the LLM branches only |
| **Sidecar (contract)** | sidecar-testing (the battery) | CLI probe (spot checks), stdio method-call (wire) |
| **Go change** | Go + tasktest layers | CLI probe after rebuild (the surface still serves) |
| **GUI** | GUI via browser | terminal protocol (the feed behind the screen) |
| **CWM pack** (type, capability, panel, gate, translations) | Go (the shipped-content tests and the census, `ktw-compose:/skill/test-go`) + `lint-universe` / `i18n --missing` | scenario case modelled on `demo-showcase` (no LLM), then GUI via browser for the tabs |
| **Prompt / LLM text** | run+trace (A/B the text) | improve-artifact loop |
| **A whole flow** (planification, a method, several sidecars) | scenario case | GUI via browser on the case's universe |

## 3. Pick by the question, not the artifact

- "does it parse / load / list?" → CLI probe — never spend LLM tokens on this
- "does the orchestration wire up?" → dry-run
- "what does the model actually do?" → run+trace (and accept stochasticity —
  repeat runs before trusting a delta)
- "does the interactive flow work?" → virtual terminal
- "what happened in that run?" → journals
- "the run HANGS / no output / can't see why?" → hung run (pprof) — capture
  the goroutine stacks while it's still hung; the journals won't show it
- "does the user see the right thing?" → GUI via browser
- "does the platform carry this from start to finish, on a real project?" →
  scenario case — and, when a stage fails, fix and replay that stage only
- "did my code change break anything?" → Go + tasktest layers
  (L1+L2 only; L3 costs tokens and runs ONLY on explicit user request)

Cost order, cheapest first: journals (free) → CLI probe / dry-run / Go →
stdio method-call / terminal protocol → virtual terminal → run+trace → GUI →
scenario case.
Start as cheap as the question allows; escalate only when the cheaper method
cannot answer it.

**Overlay artifacts test live.** When an embedded skill/task is being iterated
through the workspace overlay (authored with `target: overwrite`, not yet
promoted — `ktw-compose:/skill/overlay-iteration`), it is served at its real URI
with no rebuild between edit and test. Test it exactly as normal; a
`stale-embedded-skill`-style freshness check recognises the overlay and does not
flag it. No `make <sidecar>` until you promote.

## 4. Always end with evidence

Whatever the method: capture the output (trace file, journal path, probe
transcript, screenshot), state the verdict against what the artifact
promised, and keep the artifacts — a test whose evidence is gone proves
nothing next week. For repeated testing, package the scenario as a task per
`ktw-compose:/skill/skill-eval-method` so it re-runs unchanged.
