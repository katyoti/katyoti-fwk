# CLI UX Review

Reference for evaluating a CLI's user experience using a greenfield methodology: design the ideal CLI from user needs first, then gap-analyse reality.

---

## Purpose

Audit a CLI tool's UX by reasoning from user needs outward — never starting from the existing interface. Produces a capability inventory, greenfield command tree, gap analysis, and prioritized migration strategy.

## Scope

Input: a CLI codebase, its documentation, or a description of the tool.
Allowed tools: Read, Grep, Glob.
Output: Markdown report in the same language as the input.

## Process

### Phase 1 — Functional capability inventory

#### 1.1 Discover all capabilities

Scan the codebase and documentation to build a complete inventory of what the software can do. Group by domain.

Output format:

| # | Domain | Capability | Description | Status |
|---|--------|-----------|-------------|--------|
| 1 | [area] | [verb + noun] | [what it does for the user] | implemented / planned / idea |

Rules:
- Use user-facing language, not code terminology
- Each capability = one atomic thing a user would want to do
- Include both implemented and planned/idea-stage features (mark clearly)
- Group by domain (workspace management, content sync, configuration, diagnostics, etc.)

#### 1.2 Identify user workflows

Map the typical user journeys — sequences of capabilities used together:

| Workflow | Steps (capabilities in order) | Frequency |
|----------|-------------------------------|-----------|
| First-time setup | configure credentials → create workspace → ... | Once |
| Daily sync | check status → sync → review changes → ... | Daily |

---

### Phase 2 — Greenfield CLI design

#### 2.1 Design principles

Evaluate against these UX principles:

1. **Verb-noun consistency** — commands follow `<verb> <noun>` or `<noun> <verb>` consistently
2. **Progressive disclosure** — simple tasks are simple; advanced options don't clutter basic usage
3. **Discoverability** — a new user can find the right command via `--help` without documentation
4. **Scriptability** — machine-readable output available (`--json`, `--quiet`), exit codes meaningful
5. **Minimal surprise** — flags behave consistently across commands, naming follows conventions
6. **Composability** — commands can be chained, output of one feeds input of another
7. **Safe defaults** — destructive operations require confirmation, dry-run available

#### 2.2 Design the ideal command tree

Starting from the capability inventory, design the command structure from scratch — ignoring what currently exists:

```
<tool-name>
├── <command-group-1>
│   ├── <subcommand-a> [flags]
│   └── <subcommand-b> [flags]
└── <top-level-command> [flags]
```

For each command, specify:
- **Synopsis**: `tool command [flags] [args]`
- **Purpose**: one-line description
- **Key flags**: name, type, default, description
- **Example usages**: 2-3 real examples

#### 2.3 Flag consistency matrix

| Flag | Short | Commands | Type | Default | Description |
|------|-------|----------|------|---------|-------------|
| --format | -f | [list] | string | — | Output format |
| --output | -o | [list] | string | stdout | Output destination |
| --quiet | -q | all | bool | false | Suppress non-essential output |
| --json | | [list] | bool | false | Machine-readable output |
| --dry-run | -n | [list] | bool | false | Preview without changes |

---

### Phase 3 — Gap analysis

#### 3.1 Command mapping

| Greenfield Command | Existing Command | Status | Notes |
|-------------------|-----------------|--------|-------|
| `tool sync push` | `tool write` | rename | verb consistency |
| `tool page read <url>` | _(none)_ | **new** | not yet implemented |

Status values: `keep` / `rename` / `restructure` / `new` / `remove`

#### 3.2 Flag mapping

| Greenfield Flag | Existing Flag | Commands Affected | Change Type |
|----------------|--------------|-------------------|-------------|
| --dry-run | _(none)_ | all write ops | **add** |

#### 3.3 Breaking changes inventory

| Change | Impact | Migration Path |
|--------|--------|---------------|
| Rename `write` → `sync push` | Scripts using `write` break | Add alias for 2 versions, then deprecation warning |

#### 3.4 UX principle violations

For each of the 7 design principles, flag violations found in the current CLI with specific examples.

---

### Phase 4 — Recommendations

#### 4.1 Prioritized changes

| Priority | Change | Effort | Impact | Breaking? |
|----------|--------|--------|--------|-----------|
| 1 | [specific change] | low/medium/high | [why it matters] | yes/no |

#### 4.2 Migration strategy

- **Phase A** (non-breaking): Add aliases, new flags, new commands alongside existing ones
- **Phase B** (deprecation): Add deprecation warnings to old commands/flags
- **Phase C** (breaking): Remove old commands/flags in next major version

#### 4.3 Implementation order

Specify which changes are independent and which have dependencies.

---

### Phase 5 — Output

Produce the report with one section per phase. Include:

1. **Capability inventory** — full table
2. **User workflow map** — key journeys
3. **Greenfield command tree** — full design with flag specs
4. **Flag consistency matrix**
5. **Gap analysis** — command mapping + flag mapping + breaking changes + UX violations
6. **Prioritized recommendations** with migration strategy

## Constraints

- Always design the greenfield model before looking at the existing CLI — never derive the ideal from the current.
- Every breaking change must have an explicit migration path.
- Recommendations must reference specific commands or flags, not vague suggestions.
- Output in plain Markdown, no bold or italic.
