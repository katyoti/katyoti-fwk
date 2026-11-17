---
name: compose-usage
description: "What ktw-compose is for: composing katyoti solutions from existing sidecars plus files-only packs, and its verb surface. Load first when encountering this sidecar."
hashtags: [compose, sidecar, solution, files-only, self-creation]
category: reference
---

# compose-usage

`ktw-compose` is the **files-only creator** of katyoti's self-creation
surface: stand up a complete katyoti **solution** without writing Go — by
composing **existing sidecars** and attaching **files-only packs** (skills,
tasks, personas, references, configs) on top.

## The verb surface

All verbs are tools (`ktw-compose:compose/<verb>`); the CLI mirrors them as
flat kebab verbs.

Verbs are `<verb>-<target>` (kebab). Listing/inspecting universes + scanning
sidecar inventory is the orchestrator's (`ktw-univers`) surface, not a compose
verb.

| Verb | Does |
|---|---|
| `create-universe <name>` | create a universe (empty member list) |
| `add-member <universe> <sidecar>` | add a member (optional `--binary <path>`, validated) |
| `remove-member <universe> <sidecar>` | remove a member |
| `create-sidecar <name>` | scaffold `app/<name>/` from the canonical templates (`--include skills,tasks,personas,references,templates,config`) |
| `create-skill <sidecar> <name>` | emit a contract-valid skill stub into `embed/skills/` |
| `create-task <sidecar> <name>` | emit a flat task stub into `embed/tasks/` (`--type agent|simple|workflow`) |
| `create-persona <sidecar> <name>` | emit a persona stub into `embed/personas/` |
| `create-reference <sidecar> <path>` | emit a reference doc into `embed/references/` |
| `lint-skill` / `lint-task` / `lint-reference` / `lint-persona` / `lint-guide-recipe` / `lint-deck` | check one file (`--file`) or a published one (`--uri --exe`) against its contract, its links and its URIs |
| `lint-sidecar --exe <bin>` / `lint-universe --bin-dir <dir>` | lint everything a compiled sidecar publishes / every sidecar against the others (cross-sidecar URIs, a gate out of its provider's scope) |
| `i18n <folder\|exe> --lang fr [--missing] [--scaffold]` | what a pack still has to translate; `--scaffold` writes the `<name>.i18n.yaml` stubs |
| `inspect <uri>` | one contribution: kind, layers, provider, verdict |
| `create-content <name>` | a content FOLDER served by ktw-host (skills, tasks, references, a CWM pack) — no Go toolchain |
| `scenario …` | end-to-end scenario cases in a universe of their own (`ktw-compose:/skill/test-scenario-case`) |
| `promote <sidecar> <type>/<name>.md` | fold a workspace-overlay artifact into the sidecar's source `embed/` and delete the overlay copy |
| `apply-universe` | headless: realize a solution spec — optionally scaffold+build a new files-only sidecar, create the universe, add every member |
| `compose-universe <name>` | the interactive headline flow: pick sidecars (forms) or `--from-spec spec.yaml`, then converge on `apply-universe`. `--replace` archives an existing universe of the same name |

Rules the verbs enforce:

- **In-repo only**: file-emitting verbs run from the ktw-agents repo root
  (go.mod + app/ present) — out-of-repo emission is not supported.
- **Never overwrite**: an existing target file or sidecar folder is refused.
- Every emission reports a follow-up note: the rebuild command, and a
  WARNING when the target's `//go:embed` directive does not cover the new
  content dir (the ships-invisible gotcha).
- **Iterate without recompiling**: pass `--target overwrite` to any `create-*`
  verb to write into the workspace overlay (`.katyoti/overwrites/<sidecar>/…`)
  — the sidecar serves it live (gated by the universe's `allow_overwrites`), no
  rebuild. `promote` folds it back into source when validated. Covers
  skills / tasks / personas / references; see `ktw-compose:/skill/overlay-iteration`.
- After `create-sidecar`, add the name to the repo Makefile's APPS list (the
  note says exactly this) and build `./app/<name>`.

The emitted files are STUBS shaped by the canonical templates — author the
real content through the authoring skills (`ktw-compose:/skill/create-artifact`
routes to the per-kind skill).

## Relationship to other surfaces

- **Track B (`ktw-code` / `ktw-gocode`)** is the *code-bearing* creator path —
  for sidecars that ship Go business logic. If the request needs Go, route
  there.
- **`ktw-univers`** owns sidecar registration + universe composition on the
  bus; the universe verbs here drive the same service rather than
  reimplementing it.
- The `compose-universe` verb IS the interactive composition flow; the
  method-call behind it is `ktw-compose:compose/apply-universe` (spec in, result out).
