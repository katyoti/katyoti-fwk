---
name: code-review
description: >-
  Comprehensive code review — scans a project, selects applicable review types,
  runs them in parallel, produces a multi-file report. Covers architecture, security,
  quality, complexity, performance, tests, and more. Use when reviewing code quality.
iconName: file-search
icon: "🔍"
hashtags: [code, review, security, quality, architecture]
category: analysis
provides: [task]
---

# Code Review

Orchestrate a comprehensive code review by scanning the project, selecting applicable review types, running them in parallel, and producing a structured multi-file report.

## Run mode — one review type of a reviews run

When the reviews service runs a code review type on a module (its runner names
this skill; your input carries `review` — the type — and `guide`, the
`ktw-code:/reference/review/<name>.md` guide), you perform **that one review
only**, on that module, and answer a review result:

1. Read the module: its object (`code_packages` — the code to review), its pages
   and its features. Read the previous review of the same type under it, if any.
2. Read the guide and follow its process on the module's code packages — no
   project scan beyond them, no selection, no question to the person.
3. Write nothing: no `modules/tasks/reviews/` folder, no summary. Call complete
   with the review result — `rating` (`critical` when a finding is critical,
   `needs-attention` when one is high or medium, `good` otherwise), `summary`,
   `report` (the guide's output format, in markdown) and `findings` (each with
   `severity`, `statement`, `location` — a file and its lines —,
   `recommendation`, `evidence`, and a stable `key`: the rule it breaks, worded
   the same every time).

The list of code review types is ktw-code's `review-types/` (one per guide
below, with its dimension); the reviews service offers them on every module
that has code. The process below is for a person's whole-project review.

## Process

### Step 1: Scan the Project

Before any review, understand what we're looking at:

1. Map the project structure (directories, key files, entry points)
2. Identify the language(s), framework(s), and build tools
3. Read dependency manifests (package.json, requirements.txt, go.mod, etc.)
4. Check for linter configs, style guides, or architecture documents
5. Determine project type: application, library/package, service, CLI tool, or mixed

Present a brief summary to the user: "This is a [type] project using [languages/frameworks], with [N] source files across [M] directories."

### Step 2: Select Reviews

Determine which reviews apply and present them to the user via AskUserQuestion (multiSelect):

| Review | Apply when... | Skip when... |
|--------|--------------|--------------|
| Architecture | Always for multi-file projects | Single file review |
| Correctness | Always | Documentation-only review |
| Security | Project has user input, network, auth, or config | Pure utility with no I/O |
| Quality & Consistency | Always for multi-file projects | Single file review |
| Complexity | Always | — |
| Library Audit | Project is a library, SDK, or reusable package | Application code |
| Performance | Project has hot paths, DB queries, I/O, or scale requirements | Trivial scripts |
| Test Coverage | Project has a test suite or user requests it | Generated/config-only code |
| CLI UX | Project exposes a CLI interface | No CLI present |
| Data Model | Project has non-trivial entities, state, or data flows | Pure stateless utilities |
| Package Layout | Multi-package project with module boundaries | Single-file or tiny project |
| Threat Model | Always for any project with user input, network, auth, or stored data | Pure offline utility with no I/O |
| Red Team Architecture | ADR, design doc, or significant architectural decision exists | No design rationale present |
| Where Are We | Module/subsystem with docs, ADRs, or prior review reports; user wants a state-of-play + work plan | Single file/snippet; single-angle review requested |

Let the user select which reviews to run. Default: all applicable.

### Step 3: Run Reviews in Parallel

Each review type has a dedicated guide in the provider references directory:

- `review/architecture.md` — Structure, tech choices, dependencies, patterns, anti-patterns
- `review/correctness.md` — Bugs, logic errors, error handling, edge cases, input validation
- `review/security.md` — Injection, auth, secrets, OWASP top 10, dependency vulnerabilities
- `review/quality.md` — Readability, naming, vocabulary/structural/style consistency, documentation
- `review/complexity.md` — Einstein analysis: over-complicated and under-complicated code
- `review/library-audit.md` — API consistency, extensibility, completeness (packages only)
- `review/performance-review.md` — Algorithmic complexity, N+1 queries, caching, I/O, memory, hot paths
- `review/test-coverage-review.md` — Test quality, critical path coverage, edge cases, flakiness, test architecture
- `review/cli-ux-review.md` — Greenfield CLI UX audit: capability inventory, ideal command tree, gap analysis
- `review/data-model-review.md` — Entity design, state machines, data flows, type duplication, greenfield model
- `review/package-layout-review.md` — Responsibility domains, dependency graph, layer violations, import analysis
- `review/threat-model.md` — OWASP/CWE-mapped threat inventory, risk scoring, attack paths, prioritized test plan
- `review/red-team-architecture.md` — Adversarial stress-test of architectural decisions: claim decomposition, multi-perspective analysis, steelman + counter-argument
- `review/where-are-we.md` — META-review: intent (docs/ADRs/backlog) vs implementation vs prior review findings → state-of-play synthesis + ordered work plan

**Execution strategy**: Launch selected reviews as parallel agents. Each agent:
1. Reads its review guide (the `ktw-code:/reference/review/<name>.md` resource)
2. Reads the relevant project files for its scope
3. Produces its report following the output format in the guide

**Exception — Where Are We runs LAST**: it is a synthesis review whose inputs include the other reports from the same run (and any prior `modules/tasks/reviews/<scope>_*/` folders). Launch it only after the parallel reviews complete. It can also run alone in synthesis mode over a prior run's folder — in that case it MUST re-verify every carried-forward finding against current code (see the guide's Phase 3).

### Step 4: Write Reports

Save each review report to `modules/tasks/reviews/<scope>_YYYY-MM-DD_HH-MM/` — prefix the folder with a short scope slug when the review targets a specific module (e.g. `tasks_2026-05-06_07-56`); omit the prefix only for whole-project reviews:

```
modules/tasks/reviews/<scope>_YYYY-MM-DD_HH-MM/
  architecture.md
  correctness.md
  security.md
  quality.md
  complexity.md
  performance.md          (if applicable)
  test-coverage.md        (if applicable)
  threat-model.md         (if applicable)
  red-team-architecture.md (if applicable)
  cli-ux.md               (if applicable)
  data-model.md           (if applicable)
  package-layout.md       (if applicable)
  library-audit.md        (if applicable)
  where-are-we.md         (if applicable — written LAST, synthesizes the others)
  summary.md
```

Only include reports for reviews that were actually run.

### Step 5: Write Summary

Create `summary.md` that synthesizes all review results:

```markdown
# Code Review Summary — YYYY-MM-DD HH:MM

## Project
[Type, languages, frameworks, size]

## Health Overview

| Dimension | Rating | Key Finding |
|-----------|--------|-------------|
| Architecture | [good/needs-attention/critical] | [one-line] |
| Correctness | [good/needs-attention/critical] | [one-line] |
| Security | [good/needs-attention/critical] | [one-line] |
| Quality | [good/needs-attention/critical] | [one-line] |
| Complexity | [good/needs-attention/critical] | [one-line] |
| Performance | [good/needs-attention/critical] | [one-line, if run] |
| Test Coverage | [good/needs-attention/critical] | [one-line, if run] |
| CLI UX | [good/needs-attention/critical] | [one-line, if run] |
| Data Model | [good/needs-attention/critical] | [one-line, if run] |
| Package Layout | [good/needs-attention/critical] | [one-line, if run] |
| Threat Model | [good/needs-attention/critical] | [one-line, if run] |
| Red Team Architecture | [good/needs-attention/critical] | [one-line, if run] |
| Library API | [good/needs-attention/critical] | [one-line, if run] |

## Top Priorities

1. [Most impactful issue — with source review reference and code location]
2. [Second most impactful]
3. [Third most impactful]
4. [Fourth most impactful]
5. [Fifth most impactful]

## Strengths
- [What the code does well]

## Quick Wins
- [Easy fixes with high impact]

## Detailed Reports
See individual review files in this folder for full analysis.
```

### Step 6: Present to User

Present the summary. For each priority item, show the original code and suggested improvement inline. Point to individual review files for complete details.

## Single File / Snippet Mode

When the user provides a single file or code snippet (not a full project):

1. Skip Step 1 (project scan) — work with what's provided
2. Skip Step 2 (selection) — run all applicable reviews directly
3. Produce a **single combined output** instead of multi-file report (no modules/tasks/reviews/ folder)
4. Use this combined format:

```markdown
## Overall Assessment
[3-5 sentences: quality, biggest strength, most critical concern]

## Prioritized Recommendations
[Numbered list, most critical first]

## Detailed Feedback

---
**[CATEGORY]** — Issue title
*Severity*: Critical | High | Medium | Low

**Original:**
```[language]
// the problematic code
```

**Improved:**
```[language]
// the fixed code
```

**Rationale:** Why this matters.

---
```

## Rules

- Always scan first — never start reviewing without understanding the project
- Always let the user select reviews before running
- Every criticism must come with a concrete, actionable suggestion
- Every issue must include the file and line location
- Do not invent issues that are not present
- Prioritize by real-world impact: a security flaw outweighs a naming convention
- When suggesting library replacements, name the specific library
- Preserve original code exactly when quoting
- Reports go in `modules/tasks/reviews/<scope>_YYYY-MM-DD_HH-MM/` — never overwrite previous reviews
- Where Are We never re-lists a prior finding without re-verifying it against current code first
- The summary must reference specific review files, not repeat their content
