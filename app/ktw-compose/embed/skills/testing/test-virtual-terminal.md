---
name: test-virtual-terminal
description: "Drive a ktw binary through the ktw-test virtual terminal: spawn over HTTP, read stdout/stderr, inject stdin. Load to test interactive ask-user flows, long-running tasks, or scripted input."
advertised: false
hashtags: [testing, interactive, harness, ktw-test]
category: guide
---

# Virtual terminal (ktw-test)

An HTTP-driven pseudo-terminal: the test script plays the user.

## Lifecycle

```bash
./bin/ktw-test.exe server start      # background, port 8099 (stop/status/restart)
```

## The API

| Call | Purpose |
|---|---|
| `POST /run {"cmd":"./bin/ktw-agents.exe","args":[...]}` | spawn (optional `env`, `dir`) |
| `GET /output?since=N` | captured stdout/stderr lines (seq-numbered) |
| `POST /input {"text":"answer\n"}` | inject stdin |
| `GET /status` | running / exitCode / line count |
| `POST /kill` | terminate |

## The interactive pattern

1. `POST /run` the task (with `--raw --os`; `--if <file>` for file input).
2. Poll `GET /output` until the ask-user prompt appears — prompts without a
   trailing newline flush after ~500ms, so wait at least 1s after the LLM
   response before checking.
3. `POST /input` the scripted answer (newline-terminated).
4. Repeat 2–3 per question; finish on `GET /status` exitCode 0.

This is the engine behind autonomous skill evaluation: the scripted answers
ARE the simulated user (`ktw-compose:/skill/skill-eval-run`).

## Rules

- Windows: `os.Interrupt` is unreliable — ktw-test falls back to a hard kill
  after 5s; do not interpret that as a crash of the subject.
- Always `server stop` when done — a forgotten server holds the port and the
  last process.
