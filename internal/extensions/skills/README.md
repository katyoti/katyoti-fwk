# Skills Extension

The skills extension provides discoverable domain expertise packages for the agent loop. Each skill is a directory with a `SKILL.md` file containing instructions, references, and scripts.

## Package Layout

```
skills/
  api/             Public types — SkillManager, Skill, SkillInfo, input/output types
                   + system_prompt.go (BuildSystemPrompt — single source of truth
                   for the <skills_system> block, agent and CLI modes)
  local/           Extension wiring:
                     - Extension()           service + loaders + Cobra command + LLM hint
                     - AgentPluginExtension() agent loop only (skill_access strategy)
                     - ContributeSkillLoader  per-loader contribution
  commands/        skills Cobra root + 4 subcommands (list, get-instructions,
                   get-reference, run-script)
  internal/
    core/          SkillManager implementation, SKILL.md parsing, validation
    plugin/        skill_access AgentPlugin strategy (one-line wrapper around
                   api.BuildSystemPrompt for SystemMessage)
```

## SkillManager (Published Service)

`SkillManager` is the single published service. Registered via `PublishService` with KTSP exposure (HTTP/MCP). Auto-CLI generation by `AutoKTSPContainer` is **suppressed** for SkillManager (`WithKTSPSlotTags({"cli": "false"})`) — the CLI surface is hand-written in `commands/` so the `list` subcommand can emit the LLM-friendly `<skills_system>` block instead of the auto-generated human table.

Methods:
- `List(filter)` — L0 metadata for all skills (name, description, requires)
- `GetInstructions(name)` — L1 full SKILL.md body
- `GetReference(name, path)` — L2+ reference file content
- `RunScript(name, path, params, args)` — execute a skill script
- `GetSkill(name)` — internal, returns `Skill` instance

## Skill Interface (Internal)

`Skill` is held internally by SkillManager — not published, not in DI.

```go
type Skill interface {
    Title() SkillInfo
    Instructions() string
    Reference(path string) (string, error)
    RunScript(path string, params map[string]string, args []string) (RunScriptOutput, error)
}
```

## Progressive Discovery

| Level | What | Budget |
|-------|------|--------|
| L0 | name + description + requires | ~100 tokens per skill |
| L1 | Full SKILL.md instructions | < 5000 tokens |
| L2+ | Reference files on demand | < 2000 tokens each |

## Skill Loaders

Skills are discovered via the `SkillLoader` extension point. Loaders are contributed from bootstrap (not from the extension itself):

```go
skillslocal.ContributeSkillLoader(loader.NewEmbedFSLoader(efs, "."), "embed/skills")
skillslocal.ContributeSkillLoader(loader.NewFSLoader(userDir), ".")
```

Extra directories from `SkillManager.yaml` config are added by the factory.

## CLI Usage

Every binary that imports `skillslocal.Extension()` gains a top-level `skills` Cobra command with four subcommands. The point: an external LLM can drive the binary purely from the shell — no agent loop, no MCP, no tool API.

```sh
# Self-bootstrap: emits the <skills_system> block (LLM mode — verbs are CLI invocations)
$ ktw-example skills list

# L1: full SKILL.md body
$ ktw-example skills get-instructions impact

# L2: a reference file
$ ktw-example skills get-reference impact references/glossary.md

# Execute a script declared by a skill
$ ktw-example skills run-script impact scripts/extract.sh --param env=prod -- arg1 arg2
```

`ktw-example --help` (or any sidecar's `--help`) also includes a single line:
```
LLM hint: run `ktw-example skills list` to discover skill-based capabilities exposed by this binary.
```
appended at Bootstrap time, so an LLM that pipes the binary with no prior knowledge can find its way in.

The `skills list` block and the in-process `skill_access` agent plugin's `<skills_system>` system message are built by the same helper (`api.BuildSystemPrompt`) — agent mode emits `get_skill_instructions(...)` etc., CLI mode emits `<binary> skills get-instructions <name>` etc. A regression-locked golden test (`api/system_prompt_test.go`) prevents drift from the agent-mode baseline.

## Wiring Cheat Sheet

| Binary | Needs | Why |
|---|---|---|
| Sidecar with embedded skills (ktw-docs, ktw-example) | `skillslocal.Extension()` + `skillslocal.ContributeSkillLoader(NewEmbedFSLoader(efs, "."), "embed/skills")` + `embed/skills` in the binary's `//go:embed` directive | Discovers embedded skills, exposes the CLI surface, no agent loop |
| ktw-agents (orchestrator) | All of the above PLUS `skillslocal.AgentPluginExtension()` and explicit `application.PutService[appcore.CobraCommand](app, "root", ...)` (because ktw-agents bypasses `appcore.NewAppExt`) | Full agent loop including `skill_access` plugin |
| Sidecar without embedded skills | Nothing — adding the extension is harmless but pointless | — |

