---
name: emit-bad-skill
title: Emit a deliberately bad skill (eval fixture)
type: agent
advertised: false
description: "VERIFICATION FIXTURE — deterministically emits a KNOWN format-violating, wrong-type skill (over-long multi-sentence description, internal jargon, meta-commentary, a When-to-use section, task-shaped frontmatter) as a compose-skill object, so the eval harness can be proven to catch it (a NON-pass verdict). Not a real authoring task."
iconName: bug
icon: "🐞"
hashtags: [fixture, evaluation, harness, test]
category: development
config:
  capability: smart
  mode: worker
  plugins:
    - standard   # file-write the skill file
    - cwm         # cwm-object-create to register the compose-skill object
---

# Emit a bad skill (fixture)

You are a FIXTURE. Produce ONE deliberately defective skill so the evaluation
harness can be proven to catch it. Do exactly these steps — do not improve or
clean up the skill; its defects are the point.

## 1. Write the bad skill file

`file-write` `workspace_path: test_results/bad-trend-skill.md` with EXACTLY this
content (verbatim — every defect is intentional):

```
---
name: bad-trend
description: Research IT trends for a Dossier d'architecture section and produce a structured markdown trend document using web search tools to gather current sourced information per trend. Load when the user asks to research trends or produce a trend analysis or fill the veille technologique section with current technology landscape data.
type: agent
input-schema:
  type: object
output-schema:
  type: object
---

# Bad trend skill

There are no new tools here. This skill drives the agent.

## When to use

- when the user wants a trend analysis
```

(Defects, for the record — you do NOT fix them: description over-long + multiple
sentences; `type` + `input-schema`/`output-schema` in frontmatter = task-shaped,
wrong artifact type; "There are no…" meta-commentary; a `When to use` body
section.)

## 2. Register it as a compose-skill object

Create a `compose-skill` object so the harness can verify it, in ONE
`cwm-object-create` call — the type-specific `skill_file` goes in the content
body (the compose-skill linters read that key):

```json
{
  "module": "<the module you work in>",
  "object": {
    "type": "compose-skill",
    "capabilities": {
      "container": { "name": "Bad trend (fixture)" },
      "content": {
        "title": "Bad trend (fixture)",
        "body": { "skill_file": "test_results/bad-trend-skill.md" }
      }
    }
  }
}
```

Omit `id`: one is minted and returned.

## 3. Complete

Call **complete** with `node_id` (the created object's id) and
`skill_file: test_results/bad-trend-skill.md`. That is the whole job — an object
whose skill is knowingly broken, ready for the eval to flag.
