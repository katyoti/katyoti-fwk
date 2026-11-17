---
name: task-intent-interview
description: "Clarify what a new task must do before writing it: its job, how and by whom it is launched, its I/O contract, and which engine type fits. Load at the start of creating a task from a vague request."
advertised: false
hashtags: [task, authoring, interview, create-items]
category: guide
---

# Clarifying a task's intent

A task is launched and tracked — by a user, a GUI, or another agent — and must
return a useful result. Before writing one, pin down what it does and how it is
driven, so the `type`, the I/O contract, and the tools are chosen on purpose,
not guessed.

The interview method — open → probe → confirm, one question per round — is
`ktw-methods:/skill/interview-technique`. Load it and apply it. This skill adds
**what to ask about for a task**.

## What we must know

1. **The job + its result.** What does one run accomplish, and what does the
   caller get back? A task earns its existence by producing a concrete result a
   caller consumes — name that result.
2. **Who launches it, and how?** A user via `run`, a GUI action, or another
   agent via `delegate`? This shapes the input contract and whether it runs
   foreground or background.
3. **The input contract.** What does a caller pass in? If it is structured,
   that becomes the input contract file (`<name>.input.dsl.yaml`); if it is free text, the body/prompt consumes it.
4. **Which engine type?** Decide deliberately:
   - **`agent`** — needs to *reason*, use tools, iterate. The common case.
   - **`workflow`** — fixed multi-step orchestration (sequence / parallel /
     spawn / branch) with no per-step reasoning. Deferred sub-skill.
   - **`task`** — a single deterministic verb dispatch, no LLM.
5. **The tools / plugins.** What must the agent be able to do — read/write
   files, run shell, delegate, ask the user, fetch the web? This picks the
   `plugins:` set (`ktw-compose:/reference/artifact/task-tools.md`).
6. **Existing overlap.** Scan `tasks list` — does a task already do this or
   close to it? If so, extend it rather than add a sibling.
7. **Home.** A sidecar's `embed/tasks/` (ships in the binary, needs a rebuild)
   or `<workspace>/.katyoti/tasks/` (runtime, no rebuild)? Ask or state — never
   assume.

## Description-first

Draft the `description` during the interview: one line, *what it does and when
you'd run it*. Read it back at the summary — if it does not make the task's
purpose and trigger obvious, probe more.

## Output — the task intent brief

- **name / description** — slug + the what+when line
- **type** — agent | workflow | task (with the reason)
- **result** — what one run returns
- **input** — free text, or the structured input contract's shape
- **plugins** — the tool set the body needs
- **verdict** — extend `<existing>` or create new
- **home** — sidecar `embed/tasks/` (which) or `.katyoti/tasks/`

Confirm with the user (`request-validation` kind `approve` when it is offered, else `ask-user` confirm), then hand to
`ktw-compose:/skill/create-task`.
