---
name: authoring-lessons
description: "Hard-won task/skill authoring pitfalls — each rule discovered through a real failure (premature complete, vague prompts, lost tool names, …). Load before finalizing a task or skill."
advertised: false
hashtags: [task, skill, authoring, lessons, create-items]
category: guide
---

# Task & Skill Authoring Best Practices

Lessons learned from real task/skill development and debugging. Every rule below
was discovered through an actual failure — not speculation.

---

## 1. The agent WILL call `complete` too early

**Problem**: LLMs are completion-seekers. After 2-3 phases of a 7-phase methodology,
the model decides "good enough" and calls `complete` — especially when context fills
up from reading many files.

**Fix**: Add a **Definition of Done** checklist to TASK.md. List every required output
file by name. The agent must walk the checklist before completing. A DoD is enforceable;
prose warnings ("DO NOT complete early") are not.

```markdown
## Definition of Done
Before calling `complete`, verify EVERY item:
- [ ] `00-overview.md` exists and has content
- [ ] `01-architecture.md` exists and has content
...
```

**Why it works**: LLMs are significantly better at following checklists than at obeying
negative instructions. The checklist gives them a concrete loop: check → fix → next.

---

## 2. Phase gates must say "STOP and WRITE"

**Problem**: Soft gates like "Write `01-architecture.md`" are treated as suggestions.
The agent reads them as "I should write this eventually" and batches all research first.
When it runs out of turns, zero files have been written.

**Fix**: Gates must be imperative and unambiguous:

```markdown
**Gate — STOP and WRITE:** Call `file-write` to create `01-architecture.md`
NOW, before proceeding to Phase 3.
```

**Pattern**: research → write → next phase → research → write → next phase.
Never: research → research → research → write everything at the end.

---

## 3. TASK.md is the primary prompt — put critical rules there

**Problem**: The SKILL.md contained the quality checklist and completion requirements,
but the agent completed early without ever reaching those sections. Skills are
loaded via `get-instructions` and become part of the conversation context —
but they're seen as reference material, not directives.

**Fix**: The TASK.md is the agent's system prompt — it's the first thing seen and
always present. Critical rules (DoD, completion gates) go in TASK.md. The SKILL.md
provides the methodology. The separation is:

| Document | Role | Content |
|---|---|---|
| TASK.md | Directive (what MUST happen) | Steps, DoD, completion rules |
| SKILL.md | Methodology (how to do it) | Phases, techniques, templates |

---

## 4. Tool input schemas must have sensible defaults

**Problem**: The `ask-user` tool had `"required": ["question", "type"]` in its schema.
LLMs frequently omit optional-looking fields like `type`. Every invocation failed
twice before the model figured out to add `type: "text"`.

**Fix**: Make the schema match how LLMs actually call tools:
- Only truly required fields are `required` (e.g., `question`)
- Fields with sensible defaults use `omitempty` and code-level defaulting
- Enum fields that have a natural default should not be required

```go
Type string `json:"type,omitempty"` // default to "text" in code
```

**Corollary**: If a field fails >10% of the time because the LLM omits it,
it shouldn't be required.

---

## 5. Short output > missing output

**Problem**: When the agent runs low on context/turns, it faces a choice: skip the
remaining phases entirely, or write short documents for each. Without explicit guidance,
models skip entirely (calling `complete` with partial work).

**Fix**: Add explicit fallback instructions in TASK.md:

```markdown
If running low on context or turns, write what you have for the CURRENT phase,
then continue to the next. Short documents are better than missing documents.
Even "[Requires further analysis]" is better than a missing file.
```

---

## 6. Tool usage minimums enforce thoroughness

**Problem**: Agents take shortcuts — reading 2-3 files then inferring the rest.
The output looks plausible but contains fabricated claims not grounded in actual code.

**Fix**: Set minimum tool-call thresholds in the SKILL.md:

```markdown
## Critical Rules
- `glob` called at least 3 times
- `file-read` called at least 10 times
- `grep` called at least 5 times
```

This is replicated in the DoD as a checkable item. The agent can count its own
tool usage and self-correct if below threshold.

---

## 7. Don't duplicate code paths — one implementation per capability

**Problem**: `ask-user` had two implementations — a standalone tool (with correct
defaults and auto-generated schema) and a legacy embedded plugin (with wrong schema
and no defaults). The standalone was never wired; the legacy always ran.

**Fix**: One tool, one registration, one code path. When migrating from an older
pattern (embedded plugin → standalone tool), wire the new one AND remove the old
one in the same commit. Dead code that "still exists for backward compatibility"
becomes the code that runs forever.

**Checklist for tool migration**:
- [ ] New tool registered in bootstrap
- [ ] Old tool removed from its extension
- [ ] Plugin group (Plugins.yaml) references resolve to the new tool
- [ ] Tests pass with ONLY the new tool wired

---

## 8. Evidence-based output prevents hallucination

**Problem**: LLMs generate confident-sounding descriptions of code they never read.
Without grounding, the output is creative writing, not specification.

**Fix**: Every claim must reference a source file and line:

```markdown
Code evidence required — Every claim must reference a source file and line.
Use `file:line` format. No placeholder text — missing info documented as
"[Not found — searched: ...]".
```

The `file:line` format is both human-verifiable and machine-parseable.

---

## 9. Capability composition: declare `skills:` directly, not a skill bundle

**Problem**: Using `plugins: [augmented, standard]` loads the entire skill
infrastructure (every advertised skill, plus the web/scripts tools that the
`augmented` bundle drags in) even when the task only needs one specific
skill.

**Fix**: Declare the skills the task needs in the dedicated `skills:` list — a
sibling of `plugins:` under `config:`. It supports exact names, prefix globs,
and provider-scoped wildcards:

```yaml
config:
  plugins:
    - standard
  skills:
    - spec-extractor              # exact name
    # - cwm-*                     # all advertised skills whose name starts cwm-
    # - ktw-methods:/skill/*     # all advertised skills from one provider
```

Declaring `skills:` auto-attaches the skill plugin scoped to just those skills —
no need to also list a skill bundle. Bulk selectors (`*`, globs) match
**advertised** skills only; an exact name reaches an unadvertised one too.

---

## 10. Test on small scope first, then scale

**Pattern**: Always validate a task/skill on a small, fast target before running on
the full codebase:

1. **Small folder** (8-10 files, ~1K lines) — validates the chain works end-to-end
2. **Medium scope** (one subsystem, ~50 files) — validates phased methodology
3. **Full repo** — validates scalability and context management

Each scale reveals different failure modes:
- Small: wiring/discovery/tool errors
- Medium: phase-gate compliance, early completion
- Full: context exhaustion, output quality degradation

---

## Summary — TASK.md template

```markdown
---
name: <task-name>
type: agent
description: "<one-line description>"
config:
  mode: worker
  capability: smart
  plugins:
    - standard
  skills:
    - <skill-name>
---

# <Task Title>

<1-2 sentence role assignment>

**Load the `<skill-name>` skill first** — follow it strictly.

## Steps
1. <get input from user or context>
2. Follow the skill's phases in order
3. After EACH phase, write output before starting the next
4. Walk the DoD below before completing

## Definition of Done
Before calling `complete`, verify EVERY item:

### Output files
- [ ] <file1> exists and has content
- [ ] <file2> exists and has content
...

### Quality checks
- [ ] <quality criterion 1>
- [ ] <quality criterion 2>

### Tool usage minimums
- [ ] `glob` called at least N times
- [ ] `file-read` called at least N times
```
