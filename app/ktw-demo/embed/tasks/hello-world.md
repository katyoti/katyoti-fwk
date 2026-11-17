---
name: hello-world
description: A simple test task that greets the user and writes a greeting file
type: task
iconName: hand-wave
icon: "👋"
hashtags: [demo, greeting, simple, schema-driven-ui]
category: demo
config:
  capability: fast
  plugins:
    - standard
---

# Hello World Task

Your input arrives in the prompt inside a `=== FORM_INPUT ... === END FORM_INPUT ===`
block as a JSON object with three keys: `name` (string), `greeting`
(string, one of: Hello / Hi / Bonjour / Hola / Ciao), and `loud`
(boolean).

**You MUST extract these three values from the FORM_INPUT block.** Do
not invent defaults. Do not greet a generic "World" — the user gave
you their name. Do not use `ask-user` — every value is already in the
prompt.

Steps:

1. Read the FORM_INPUT block and pull out `name`, `greeting`, `loud`.
2. Compose the greeting line: `<greeting>, <name>!` (e.g. `Bonjour, Alice!`).
   If `loud` is `true`, upper-case the whole line (`BONJOUR, ALICE!`).
3. Call `file-write` to create `greeting.md` with the first line
   `# <greeting line>`.
4. Call `complete` with:
   - `greeting_file`: the path you wrote
   - `rendered_greeting`: the greeting line as markdown, bolded —
     `**<greeting line>**` (post-loud transform)
   - `executed_at`: the current date-time as ISO 8601 (e.g.
     `2026-06-11T14:30:00Z`); use today's date from your context
   - `was_loud`: the boolean `loud` value you extracted

Worked example for `{"name": "Alice", "greeting": "Bonjour", "loud": true}`:
- greeting line: `BONJOUR, ALICE!`
- file content: `# BONJOUR, ALICE!`
- complete payload: `{"greeting_file": "greeting.md",
  "rendered_greeting": "**BONJOUR, ALICE!**",
  "executed_at": "<now as ISO 8601>", "was_loud": true}`
