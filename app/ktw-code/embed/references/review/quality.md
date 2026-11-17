# Quality & Consistency Review

## Purpose

Assess code readability, naming, style consistency, and documentation quality across the codebase.

## Scope

Reads: source code, documentation files, configuration. Focuses on how well the code communicates its intent and how consistent the codebase is with itself.

## Process

### 1. Readability

- **Naming quality**: are variables, functions, classes named clearly and consistently?
- **Function length**: are functions focused on a single task?
- **Code organization within files**: logical grouping, clear flow
- **Comment quality**: comments explaining why (not what)? Misleading comments present?
- **Magic numbers/strings**: unexplained constants that should be named

### 2. Vocabulary Consistency

Scan all locations where domain terms appear (code, docs, comments, UI labels):
- Synonyms used for the same concept (e.g., "compile" vs "emit" vs "generate")
- Inconsistent capitalization or spelling of the same term
- Technical terms that differ between documentation and code

### 3. Structural Consistency

Compare naming and organization patterns across the project:
- Folder naming patterns (kebab-case vs snake_case vs camelCase, singular vs plural)
- Code organization across similar components (same layout?)
- File naming conventions within and across directories

### 4. Code Style Consistency

- Naming conventions (camelCase, snake_case, PascalCase) — uniform?
- File structure patterns (imports order, section layout)
- Export patterns (default vs named, barrel files)
- Error handling patterns — same approach everywhere?
- Similar operations solved differently in different places

### 5. Documentation Completeness

- Are all public APIs documented?
- Are all features documented in the same way (same template, same depth)?
- Are documentation claims accurate vs actual code behavior?
- Broken links or missing references

### 6. Best Practices & Idioms

- Language conventions followed?
- Framework idioms used correctly?
- Linting compliance (if linter config present)
- Consistent formatting

## Output Format

```markdown
# Quality & Consistency Review

## Consistency Score
[Excellent / Good / Needs Work / Inconsistent]

## Readability Issues
| Location | Issue | Severity | Fix |
|----------|-------|----------|-----|
| [file:line] | [description] | high/medium/low | [suggestion] |

## Vocabulary Inconsistencies
| Term A | Term B | Where | Fix |
|--------|--------|-------|-----|
| [term] | [synonym used elsewhere] | [file locations] | [which to standardize on] |

## Structural Inconsistencies
[Patterns that don't align, with specific examples]

## Style Inconsistencies
[Code style variations across the codebase]

## Documentation Gaps
[Missing or incomplete documentation, with what's expected]

## Recommendations
[Prioritized list — most impactful quality improvements]
```
