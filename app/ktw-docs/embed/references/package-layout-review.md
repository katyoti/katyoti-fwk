# Package Layout Review

Reference for evaluating a codebase's package and module architecture using a greenfield methodology: design the ideal package structure from responsibility domains first, then gap-analyse reality.

---

## Purpose

Audit a codebase's package layout by reasoning from domain boundaries and dependency direction — never starting from the existing structure. Produces a responsibility domain map, greenfield package tree, dependency graph, gap analysis, and a safe refactoring strategy.

## Scope

Input: a software codebase (any language) or an architecture document describing its package/module structure.
Allowed tools: Read, Grep, Glob.
Output: Markdown report in the same language as the input.

## Process

### Phase 1 — Functional capability inventory

#### 1.1 Discover all capabilities

Scan the codebase to build a complete inventory of what the software does, grouped by functional area.

| # | Area | Capability | Description | Status |
|---|------|-----------|-------------|--------|
| 1 | [area] | [what it does] | [details] | implemented / planned / idea |

Rules:
- Focus on functional areas, not existing code structure
- Each capability = one cohesive piece of functionality
- Think in terms of responsibilities, not files

#### 1.2 Identify responsibility domains

From the capability inventory, identify distinct responsibility domains:

| Domain | Responsibilities | Key Abstractions |
|--------|-----------------|------------------|
| [domain name] | [what this domain owns] | [core types, interfaces] |

Rules:
- Each domain has a clear, non-overlapping responsibility
- A domain owns its data types and operations on them
- Domains communicate through well-defined interfaces

---

### Phase 2 — Greenfield package design

#### 2.1 Design principles

Evaluate the design against these architecture principles:

1. **Single responsibility** — each package has one reason to change
2. **Dependency direction** — dependencies flow in one direction (no cycles)
3. **Stability principle** — depend on stable abstractions, not volatile implementations
4. **Interface segregation** — packages expose minimal interfaces to consumers
5. **Zero-import core** — domain model packages have zero internal imports
6. **Layering** — clear layers: domain → application → infrastructure → presentation
7. **Testability** — packages can be tested in isolation with mock dependencies

#### 2.2 Design the ideal package tree

Starting from the responsibility domains, design the package structure from scratch — ignoring what currently exists:

```
internal/
├── <package-a>/          # [responsibility]
│   ├── types.go
│   ├── operations.go
│   └── interfaces.go
├── <package-b>/          # [responsibility]
└── <package-c>/
    └── <sub-package>/
```

For each package:

| Package | Responsibility | Exports | Internal Imports | Layer |
|---------|---------------|---------|-----------------|-------|
| `domain` | [what it owns] | [key types/interfaces] | none | domain |
| `application` | [what it owns] | [key types/interfaces] | domain | application |
| `infra` | [what it owns] | [key types/interfaces] | domain | infrastructure |
| `cli` | [what it owns] | [key types/interfaces] | application | presentation |

#### 2.3 Dependency graph

Draw the dependency graph:

```
[presentation] → [application] → [domain]
                       ↓
               [infrastructure]
```

Rules for the graph:
- No circular dependencies
- Domain layer has zero internal imports
- Infrastructure adapters depend on domain interfaces (dependency inversion)
- Presentation depends on application layer only, never on infrastructure directly

#### 2.4 Interface boundaries

Define the key interfaces between packages:

| Interface | Defined In | Implemented By | Purpose |
|-----------|-----------|----------------|---------|
| `PageStore` | domain | infrastructure/store | CRUD on pages |
| `RemoteAPI` | domain | infrastructure/api | External API access |

---

### Phase 3 — Gap analysis

#### 3.1 Package mapping

Map existing packages to the greenfield design:

| Greenfield Package | Existing Package(s) | Status | Notes |
|-------------------|-------------------|--------|-------|
| `domain/page` | `workspace` (partial) | restructure | extract pure domain types |
| `sync` | `sync` + `cli` (partial) | merge + refactor | CLI has leaked business logic |

Status values: `match` / `rename` / `restructure` / `merge` / `split` / `missing` / `remove`

#### 3.2 Responsibility leaks

| Responsibility | Should Be In | Currently In | Severity | Fix |
|---------------|-------------|-------------|----------|-----|
| [what] | [ideal package] | [actual package(s)] | High/Medium/Low | [action] |

#### 3.3 Dependency violations

| Violation | From → To | Why It's Wrong | Fix |
|-----------|----------|---------------|-----|
| Circular dep | A → B → A | [explanation] | extract interface / move code |
| Layer skip | CLI → infrastructure | bypasses application layer | route through application |
| Duplicated type | Type in A and B | [which is canonical] | keep one, import the other |

#### 3.4 Import graph comparison

| Package | Greenfield Imports | Current Imports | Delta |
|---------|-------------------|-----------------|-------|
| `sync` | domain, api | domain, api, config, cli | remove cli dependency |

---

### Phase 4 — Recommendations

#### 4.1 Prioritized changes

| Priority | Change | Effort | Impact | Risk |
|----------|--------|--------|--------|------|
| 1 | [specific refactor] | low/medium/high | [why it matters] | [what could break] |

#### 4.2 Refactoring strategy

Specify the safe order of operations:

1. **Extract first** — move code without changing behavior (mechanical refactoring)
2. **Then restructure** — change package boundaries, introduce interfaces
3. **Then clean up** — remove duplication, align naming, add package documentation

For each step: what moves where, what tests verify the move, rollback plan.

#### 4.3 Dependency rule enforcement

How to prevent regression after refactoring:

| Rule | Enforcement | Tool / Method |
|------|-------------|--------------|
| No circular imports | Compiler (Go) / linter (JS/TS) | automatic |
| Layer violations | Architecture tests | go-arch-lint, dependency-cruiser, etc. |
| Zero-import domain | CI check | grep on import blocks |

---

### Phase 5 — Output

Produce the full report with one section per phase. Include the dependency graph (text representation or Mermaid). Conclude with a synthesis table of all violations by severity and a numbered priority recommendation list.

## Constraints

- Always design the greenfield package structure before analysing the existing code — never derive the ideal from the current.
- Every dependency violation must have a concrete fix.
- The refactoring strategy must specify a safe order — no step should break the build if performed in isolation.
- Output in plain Markdown. Mermaid diagrams are the exception for dependency graphs.
