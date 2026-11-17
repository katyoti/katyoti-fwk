---
name: test-cli-probe
description: >-
  Test a sidecar's CLI surface directly — tools, resources, skills — with
  zero LLM cost. Load to verify an artifact parses, lists, loads, and that
  the llm/sc surfaces serve what the binary promises.
advertised: false
hashtags: [testing, cli, probe]
category: guide
---

# CLI probe

Deterministic probes against a built binary (`$BIN` = `./bin/<sidecar>.exe`).
No server, no LLM, instant verdicts.

## The probes

Run these read-only introspection calls against the LLM-facing surface and the
operator surface (each named by its wire method):

- **`info`** — identity + conventions + tools + skills in one call.
- **`tools/list`** — the tool inventory.
- **`tools/describe <tool-uri>`** — one tool's schema.
- **the resource-list method `--type <t>`** — resources of one type.
- **the resource-read method `<uri>`** — content for skills, references, any type.
- **the served skills listing** — the `<skills_system>` block as served.

## What each answers

- **A new/edited skill**: appears in the served skills listing (advertised) or
  loads by name when you read the `<provider>:/skill/<name>` resource (sub-skill).
  KO if the strict frontmatter fails — the skill silently vanishes from the list.
- **A reference**: reading the `<provider>:/reference/<path>` resource returns
  the file content. KO if empty — every URI a skill names must resolve.
- **A task**: appears in the task listing with the right provider.
- **A tool**: the describe method returns a schema; the call method with minimal
  valid input returns a result, not an error envelope.

## Rules

- Rebuild before probing (`go build -o bin/<sidecar>.exe ./app/<sidecar>`) —
  probing a stale binary tests yesterday's code. On Windows always build with
  the `.exe` suffix: a stale extension-less twin shadows the new binary.
- `llm` and `sc resources` twins must emit identical JSON — a mismatch is a
  parity defect worth reporting.
- For the FULL battery (every verb × every resource type, KO tables), load
  `ktw-compose:/skill/sidecar-testing`.
