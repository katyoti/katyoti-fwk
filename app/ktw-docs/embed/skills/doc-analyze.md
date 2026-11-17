---
name: doc-analyze
description: >-
  Read-only document analysis, review, and explanation skill. Never modifies the
  input document. Analyzes user intent, proposes matching analysis type(s) via
  interactive selection, then dispatches to specialized guides. Supports: prose
  quality analysis (standard ratings, JSON output, Pinker's Sense of Style),
  technical document analysis, documentation corpus analysis, architecture/design
  review, tool documentation explanation, requirements review (SRS/BRS/PRD),
  document diff/comparison, argument and logic analysis, dynamic wisdom
  extraction from any content, and code architecture reviews (CLI UX, data
  model & state, package layout). Use when the user wants to analyze, review,
  critique, evaluate, rate, compare, extract wisdom from, or explain any
  document, text, or codebase.
provides: [task]
---

# Document Analysis

**Input:** a file path, raw text, or an analysis type.

Orchestrate read-only document analysis by understanding the user's intent, selecting the right analysis type, and dispatching to specialized reference guides. This skill NEVER modifies the input document.

## Process

### Step 1 — Understand Intent

Analyze `$ARGUMENTS` and the user's request. Classify into one or more analysis categories:

| Category | Analysis Types | Trigger keywords |
|----------|---------------|------------------|
| Prose Analysis | prose-standard, prose-json, prose-pinker | writing quality, prose, critique, rate, evaluate writing |
| Technical Analysis | tech-doc, tech-corpus | analyze document, check consistency, corpus review, documentation audit |
| Review | design-review | architecture review, design review, system design critique |
| Explanation | explain-docs | explain tool, clarify documentation, make docs usable |
| Requirements | requirements-review | requirements review, SRS review, PRD review, BRS review, check requirements, user stories audit |
| Comparison | doc-diff | compare documents, diff, what changed, v1 vs v2, before and after, two versions |
| Reasoning | argument-analysis | analyze argument, logical analysis, fallacies, reasoning, rhetoric, position paper, logic check |
| Wisdom Extraction | extract-wisdom | extract wisdom, key takeaways, what's interesting, analyze video, analyze podcast, extract insights, what did I miss |
| Code Architecture | cli-ux-review, data-model-review, package-layout-review | CLI UX review, command design, data model review, state machine analysis, package layout, module architecture, dependency graph, code structure review |

### Step 2 — Select Analysis Types

Present the proposed analysis type(s) to the user via `AskUserQuestion` (multiSelect), grouped by category.

Pre-select types that match the detected intent. If intent is unambiguous (user explicitly said "review this design"), skip selection and confirm directly.

Available types with descriptions:

| Type | Description |
|------|-------------|
| prose-standard | A-F ratings on novelty, clarity, prose with Markdown report |
| prose-json | Same A-F ratings as prose-standard but output as structured JSON |
| prose-pinker | Full Pinker Sense of Style framework with scored assessment |
| tech-doc | Single technical Markdown document analysis with cross-reference checks |
| tech-corpus | Structural and consistency analysis of a documentation folder/corpus |
| design-review | Architecture and system design review across 8 dimensions |
| explain-docs | Transform tool documentation into a clear, structured guide |
| requirements-review | Completeness, testability, ambiguity, and conflict analysis for SRS/BRS/PRD/user stories |
| doc-diff | Semantic comparison of two document versions or competing specs |
| argument-analysis | Logical structure mapping, fallacy detection, evidence quality, rhetorical analysis |
| extract-wisdom | Dynamic wisdom extraction with content-adaptive sections — for videos, podcasts, articles |
| cli-ux-review | CLI UX audit via greenfield methodology — capability inventory, ideal command tree, gap analysis |
| data-model-review | Data model, entity lifecycle, state machine, and data flow audit via greenfield methodology |
| package-layout-review | Package/module architecture audit — responsibility domains, dependency graph, violation analysis |

### Step 3 — Dispatch to References

Each analysis type has a dedicated guide in the provider references directory:

- `prose-standard.md` — A-F ratings on novelty, clarity, prose with Markdown report
- `prose-json.md` — Same A-F ratings as prose-standard but output as structured JSON
- `prose-pinker.md` — Full Pinker Sense of Style framework with scored assessment
- `tech-doc.md` — Single technical Markdown document analysis with cross-reference checks
- `tech-corpus.md` — Structural and consistency analysis of a documentation folder/corpus
- `design-review.md` — Architecture and system design review across 8 dimensions
- `explain-docs.md` — Transform tool documentation into a clear, structured guide
- `requirements-review.md` — Completeness, testability, ambiguity, and conflict analysis for SRS/BRS/PRD/user stories
- `doc-diff.md` — Semantic comparison of two document versions or competing specs
- `argument-analysis.md` — Logical structure mapping, fallacy detection, evidence quality, rhetorical analysis
- `extract-wisdom.md` — Dynamic wisdom extraction with content-adaptive sections
- `cli-ux-review.md` — CLI UX audit via greenfield methodology — capability inventory, ideal command tree, gap analysis
- `data-model-review.md` — Data model, entity lifecycle, state machine, and data flow audit via greenfield methodology
- `package-layout-review.md` — Package/module architecture audit — responsibility domains, dependency graph, violation analysis

**Execution strategy**: Launch selected types as parallel agents. Each agent:
1. Reads its guide (the `ktw-docs:/reference/<type>.md` resource named above)
2. Reads the relevant input document(s) for its scope
3. Produces its analysis following the output format in the guide

### Step 4 — Present Results

After all agents complete:
1. Present each analysis report to the user.
2. For multi-analysis requests, present reports in sequence with clear separation.
3. Highlight key findings and priority recommendations across all analyses.

## Rules

- NEVER modify the input document — all output is a separate report or analysis.
- Always let the user select analysis types before dispatching (unless intent is unambiguous).
- Each reference guide is self-contained — do not mix instructions from different references.
- For multi-type requests, produce separate outputs per type.
- Every analysis must be thorough — never skip categories, checks, or dimensions defined in the reference guide.
