---
name: doc-fix
description: >-
  Document fixing and improvement skill. Modifies existing documents in place
  to fix formatting, improve writing quality, or clean up broken text. Analyzes
  user intent, proposes matching fix type(s) via interactive selection, then
  dispatches to specialized guides. Supports: academic writing improvement,
  broken/malformatted text cleanup, and heading/formatting correction for French
  documents. Use when the user wants to fix, improve, clean up, reformat, or
  polish an existing document or text.
provides: [task]
---

# Document Fix

**Input:** a file path or text to fix.

Orchestrate document fixing and improvement by understanding the user's intent, selecting the right fix type(s), and dispatching to specialized reference guides that modify the document in place.

## Process

### Step 1 — Understand Intent

Analyze `$ARGUMENTS` and the user's request. Classify into one or more fix categories:

| Category | Fix Type | Trigger keywords |
|----------|----------|------------------|
| Writing Quality | improve-writing | improve, refine, academic, polish, scholarly |
| Text Cleanup | fix-text | fix broken text, clean up, malformatted, OCR artifacts |
| Document Formatting | fix-formatting | headings, formatting, French capitalization, structure |

### Step 2 — Select Fix Types

Present the proposed fix type(s) to the user via `AskUserQuestion` (multiSelect), grouped by category.

Pre-select types that match the detected intent. If intent is unambiguous (user explicitly said "fix the formatting"), skip selection and confirm directly.

Available types with descriptions:

| Type | Description |
|------|-------------|
| improve-writing | Refine text into professional academic/scientific language with improved clarity, coherence, and formal tone |
| fix-text | Clean up broken or malformatted text by fixing line breaks, capitalization, punctuation, and paragraph structure |
| fix-formatting | Correct heading capitalization, section structure, and formatting in French Markdown documents |

### Step 3 — Dispatch to References

Each fix type has a dedicated guide in the provider references directory:

- `improve-writing.md` — Refine text into professional academic language, produce list of changes
- `fix-text.md` — Clean up broken/malformatted text preserving all content
- `fix-formatting.md` — Correct headings and structure in French Markdown documents

**Execution strategy**: Launch selected fix types as parallel agents. Each agent:
1. Reads its guide (the `ktw-docs:/reference/<type>.md` resource named above)
2. Reads the target document or text
3. Applies its fixes directly to the document in place
4. Produces its summary of changes made

### Step 4 — Present Summary

After all agents complete:
1. Summarize changes made by each fix type.
2. List all modifications with before/after examples where applicable.
3. Confirm the document has been updated in place.

## Rules

- Always let the user select fix types before dispatching (unless intent is unambiguous).
- Each fix type focuses on its specific aspects — do not mix instructions from different references.
- Each fix type modifies the document in place — do not create copies.
- For multi-type requests, agents work on different aspects of the same document.
- Report all changes made to the user.
