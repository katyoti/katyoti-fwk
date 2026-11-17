---
name: code-explain
description: >-
  Explain a project, codebase, or file in depth. Fast-scans to discover what exists,
  lets the user choose what to explain, then generates focused explanations. Use when
  onboarding onto an unfamiliar codebase or understanding a project's architecture.
iconName: code
icon: "💡"
hashtags: [code, explanation, architecture, onboarding]
category: analysis
provides: [task]
---

# Code Explain

Explain a project or codebase by fast-scanning what exists, letting the user choose what to explain, and running focused explanations in parallel.

## Process

### Step 1: Fast Discovery

Perform a quick scan to understand what the project contains. Do NOT read file contents in depth — just check for existence:

**Code:**
- Source code directories (src/, lib/, app/, pkg/, etc.)
- Languages present (file extensions)
- Entry points (main.*, index.*, app.*, server.*)

**Documentation:**
- README files
- docs/ folder
- API documentation
- Architecture docs

**Configuration:**
- Package manifests (package.json, requirements.txt, go.mod, etc.)
- Build configs (webpack, vite, tsconfig, Makefile, etc.)
- Environment files (.env, .env.example)
- Docker/container files
- CI/CD configs (.github/workflows/, .gitlab-ci.yml)

**Tools & AI/LLM:**
- `.claude/` directory, `CLAUDE.md` — Claude Code integration
- `.cursor/`, `.cursorrules` — Cursor AI integration
- `.aider*` — Aider configuration
- `.github/copilot*` — GitHub Copilot
- Linter/formatter configs
- Pre-commit hooks

**Security indicators:**
- Auth-related code/middleware
- .gitignore patterns for secrets
- Security headers configuration

Present a discovery summary:

```
Project discovery:
- Code: [languages found], [N] source directories
- Docs: [README, docs/, API docs — what exists]
- Config: [N] config files ([list key ones])
- Tools: [linters, CI/CD, AI tools found]
- Security: [auth mechanism spotted / none visible]
```

### Step 2: Select Explanations

Based on discovery, determine which explanations are applicable and present them to the user via AskUserQuestion (multiSelect):

| Explanation | Apply when... | Skip when... |
|------------|---------------|--------------|
| Project Overview | Always | Single file input |
| Tech Stack | Package manifest exists | No dependencies |
| Architecture | Multi-module project | Single file / trivial script |
| Security Model | Auth code, secrets config, or security headers found | No security indicators |
| Tooling & AI | CI/CD, linters, or AI tool configs found | No tooling configs |
| Configuration | Config files exist | No config files |

Let the user select. Default: all applicable.

### Step 3: Run Explanations in Parallel

Each explanation type has a dedicated guide — a `ktw-code:/reference/<guide>.md` resource. Read the guide by its URI.

Available guides:

- `ktw-code:/reference/project-overview.md` — What is this, structure, entry points, how to run
- `ktw-code:/reference/tech-stack.md` — Languages, frameworks, libraries, build tools, versions
- `ktw-code:/reference/tooling.md` — Dev tools, CI/CD, testing, AI/LLM integration
- `ktw-code:/reference/configuration.md` — Config files, env vars, settings, feature flags
- `ktw-code:/reference/spec-extraction.md` — Full spec extraction (9 documents, phased methodology)

**Execution strategy**: For each selected explanation:
1. Read the guide (`ktw-code:/reference/<guide>.md`)
2. Read the relevant project files for its scope
3. Produce the explanation following the output format in the guide

### Step 4: Write Reports

Save each explanation to `docs/explanations/YYYY-MM-DD_HH-MM/`:

```
docs/explanations/YYYY-MM-DD_HH-MM/
  project-overview.md
  tech-stack.md
  architecture.md
  security.md
  tooling.md
  configuration.md
  summary.md
```

Only include explanations that were actually generated.

### Step 5: Write Summary

Create `summary.md` that synthesizes all explanations into a quick-reference onboarding guide:

```markdown
# Project Explanation Summary — YYYY-MM-DD HH:MM

## In One Paragraph
[What this project is, what it does, who it's for]

## Quick Reference

| Aspect | Key Facts |
|--------|-----------|
| Type | [application/library/service/CLI] |
| Languages | [list] |
| Frameworks | [list] |
| Database | [if any] |
| Auth | [mechanism or "none"] |
| CI/CD | [system or "none"] |
| AI Tools | [list or "none"] |

## Project Structure
```
[Annotated top-level tree]
```

## Key Commands
| Command | Purpose |
|---------|---------|
| [command] | [what it does] |

## Where to Look
- **To understand the domain**: [key files/docs]
- **To understand the architecture**: [key files]
- **To understand the config**: [key files]
- **To understand the security**: [key files]

## Detailed Explanations
See individual files in this folder for complete analysis.
```

### Step 6: Present to User

Present the summary as an onboarding guide. Point to individual explanation files for depth.

## Single File / Snippet Mode

When the user provides a single file, snippet, or security tool output (not a full project):

1. Skip discovery and selection — work with what's provided
2. Determine the input type:
   - **Source code**: explain purpose, logic flow, key constructs, dependencies, edge cases, potential issues
   - **Security tool output**: explain summary, findings with severity, risk assessment, remediation steps
   - **Configuration**: explain what it controls, each significant setting, defaults vs overrides, security implications, setting interactions
3. If the user asked a specific question, answer it directly with reference to the analyzed input
4. Produce a single inline explanation (no docs/explanations/ folder)

## Rules

- Fast discovery first — never deep-read files before knowing what exists
- Always let the user select explanations before running
- Explain what IS, not what SHOULD BE (that's for code-review)
- Use concrete examples and code references, not abstract descriptions
- When explaining patterns, briefly explain the pattern itself for readers unfamiliar with it
- Reports go in `docs/explanations/YYYY-MM-DD_HH-MM/` — never overwrite previous explanations
- The summary should work as a standalone onboarding guide

## Spec Extraction Mode

When the user asks for a "spec extraction" or "structured specification",
read the `ktw-code:/reference/spec-extraction.md` reference.
This provides a phased methodology for producing 9 specification documents
(overview, architecture, components, APIs, dependencies, configuration, patterns, data model, observations).
