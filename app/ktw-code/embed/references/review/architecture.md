# Architecture Review

## Purpose

Evaluate project structure, technology choices, component design, dependencies, and design patterns.

## Scope

Reads: project config files, entry points, directory structure, dependency manifests, module boundaries. Focuses on structural decisions, not implementation details.

## Process

### 1. Project Structure

- Map the directory structure (top 3 levels)
- Identify module/package boundaries
- Assess naming conventions and organization consistency
- Check for clear separation of concerns

### 2. Technology Choices

For each major technology/framework/library:
- Is it appropriate for the project's goals?
- Is it actively maintained?
- Are there simpler alternatives that would suffice?
- Are deprecated or end-of-life technologies in use?

### 3. Build vs Buy

- Identify code that reimplements functionality available in established libraries
- For each case, name the library that would replace it and why
- Conversely, flag heavy dependencies used for trivial tasks

### 4. Dependency Analysis

- Read dependency manifests (package.json, requirements.txt, go.mod, etc.)
- Identify heavy dependencies used for minor features
- Check for conflicting or redundant dependencies
- Assess dependency tree depth and supply chain risk

### 5. Design Patterns

- Identify patterns in use (Factory, Observer, Strategy, Repository, etc.)
- Evaluate whether they are applied correctly and are the right choice
- Identify places where a known pattern would reduce complexity but is absent
- Flag patterns misapplied or used dogmatically without clear benefit

### 6. Anti-Patterns

Actively hunt for:
- God classes / God functions
- Spaghetti code (tangled control flow)
- Shotgun surgery (one change requires touching many files)
- Feature envy (a class using another class's data more than its own)
- Inappropriate intimacy (classes knowing too much about each other)
- Cargo cult programming (code copied without understanding)
- Golden hammer (forcing one tool/pattern onto every problem)
- Leaky abstractions (implementation details bleeding through interfaces)
- Copy-paste programming (duplicated logic)

### 7. Component Coupling

- Assess coupling between components (tight vs loose)
- Check for circular dependencies
- Evaluate module interfaces — are boundaries clean?

## Output Format

```markdown
# Architecture Review

## Project Structure
[Directory map, module boundaries, organization assessment]

## Technology Choices
| Technology | Purpose | Assessment | Concern |
|-----------|---------|------------|---------|
| [tech] | [what for] | [appropriate/questionable/overkill] | [if any] |

## Build vs Buy
- **Should use library**: [custom code] -> [library recommendation]
- **Over-reliance**: [heavy dependency for trivial use]

## Dependencies
| Dependency | Purpose | Risk |
|-----------|---------|------|
| [dep] | [what for] | [none/outdated/heavy/redundant] |

## Design Patterns
- **Well-applied**: [list with assessment]
- **Missing**: [where a pattern would help]
- **Misapplied**: [where a pattern is forced]

## Anti-Patterns
| Anti-Pattern | Location | Impact | Fix |
|-------------|----------|--------|-----|
| [name] | [file:line] | [description] | [suggestion] |

## Recommendations
[Prioritized list — most impactful architectural improvements]
```
