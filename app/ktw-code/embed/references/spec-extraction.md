# Specification Extraction Methodology

> Reference for code-explain skill — addressed as `ktw-code:/reference/spec-extraction.md`

A structured approach to reverse-engineering a full specification from source code
or documentation. Input is a folder. Output is a set of `.md` files in a new directory.

## Critical Rules

1. **Read everything, assume nothing** — Use `glob`, `file-read`, and `grep` to discover and read actual code. Never infer structure from file names alone.
2. **One document per concern** — Each output file covers one aspect (architecture, APIs, dependencies, etc.). Never produce one giant file.
3. **Code evidence required** — Every claim must reference a source file and line. Use `file:line` format.
4. **Structured markdown** — Use tables, code blocks, and headings consistently. Every document starts with a summary paragraph.
5. **No opinions** — Describe what IS, not what SHOULD BE. Observations go in a separate `observations.md`.
6. **Minimum tool usage** — You MUST call:
   - `glob` at least **3 times** to discover file structure
   - `file-read` at least **10 times** on key files
   - `grep` at least **5 times** to find patterns and cross-references
7. **WRITE AFTER EACH PHASE — MANDATORY** — Each phase ends with a Gate that says
   "Write `XX-name.md`". You MUST call `file-write` to create that file BEFORE
   starting the next phase. Do NOT batch all research first and write at the end —
   that guarantees incomplete output if you run out of turns. The rule is:
   **research → write → next phase → research → write → next phase.**
   If you skip a write, the entire phase's work is lost.

## Output Structure

Create a folder `<project-name>-spec/` containing:

```
<project-name>-spec/
├── 00-overview.md           # Project summary, purpose, tech stack
├── 01-architecture.md       # High-level architecture, layers, data flow
├── 02-components.md         # Component/module inventory with responsibilities
├── 03-api-surface.md        # Public APIs, interfaces, exported types
├── 04-dependencies.md       # External deps, internal package graph
├── 05-configuration.md      # Config files, env vars, settings
├── 06-patterns.md           # Design patterns identified in the code
├── 07-data-model.md         # Key data structures and their relationships
├── 08-observations.md       # Gaps, inconsistencies, technical debt (neutral)
└── README.md                # Index linking all documents
```

## Research Phases

### Phase 1: Discover project structure

1. `glob("**/*.go")` or appropriate extension — count files, identify main packages
2. `glob("**/go.mod")` or `**/package.json` — identify module boundaries
3. `glob("**/*.yaml" OR "**/*.yml" OR "**/*.json")` — find config files
4. `file-read` the root README, go.mod/package.json, main entry point
5. `file-read` any existing architecture docs

**Gate — STOP and WRITE:** Call `file-write` to create `00-overview.md` NOW, before proceeding to Phase 2.

### Phase 2: Analyze architecture

1. Identify entry points (`main.go`, `cmd/`, `app/`)
2. `file-read` key packages — look at directory structure and package docs
3. `grep` for interface definitions — `"type .* interface"` 
4. `grep` for key patterns — dependency injection, middleware, event bus
5. Map the dependency graph between internal packages

**Gate — STOP and WRITE:** Call `file-write` to create `01-architecture.md` NOW, before proceeding to Phase 3.

### Phase 3: Inventory components

1. For each major package/directory:
   - `file-read` the main file (often named after the package)
   - Document: purpose, public types, key functions
2. Group into logical components (not just directories)
3. Note which components depend on which

**Gate — STOP and WRITE:** Call `file-write` to create `02-components.md` NOW, before proceeding to Phase 4.

### Phase 4: Extract API surface

1. `grep` for exported types, interfaces, functions
2. For each public interface:
   - Document methods with signatures
   - Note implementations
3. For each key exported function:
   - Document signature, purpose, parameters

**Gate — STOP and WRITE:** Call `file-write` to create `03-api-surface.md` NOW, before proceeding to Phase 5.

### Phase 5: Map dependencies and config

1. Parse `go.mod` / `package.json` for external dependencies
2. `grep` for import patterns to build internal dependency graph
3. `glob` + `file-read` config files (YAML, JSON, env)
4. Document env vars (`grep` for `os.Getenv`, `envkey:`, etc.)

**Gate — STOP and WRITE:** Call `file-write` to create `04-dependencies.md` AND `05-configuration.md` NOW, before proceeding to Phase 6.

### Phase 6: Identify patterns and data model

1. `grep` for common patterns: strategy, factory, observer, middleware
2. Identify key data structures — read type definitions
3. Map relationships between types

**Gate — STOP and WRITE:** Call `file-write` to create `06-patterns.md` AND `07-data-model.md` NOW, before proceeding to Phase 7.

### Phase 7: Observations and index

1. Note inconsistencies, gaps, dead code, naming issues
2. Write `08-observations.md` (neutral, factual)
3. Write `README.md` linking all documents with summaries

**Gate — STOP and WRITE:** Call `file-write` to create `08-observations.md` AND `README.md` NOW. Verify all 9+1 files exist, then call `complete`.

## Quality Checklist

Before completing, verify:
- [ ] Every `.md` file starts with a 1-2 sentence summary
- [ ] File references use `file:line` format
- [ ] Tables are used for inventories (components, APIs, deps)
- [ ] Code blocks show actual code from the project (not invented)
- [ ] No placeholder text — missing info documented as "[Not found — searched: ...]"
- [ ] README.md links and summarizes all documents
