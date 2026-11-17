# Correctness Review

## Purpose

Identify bugs, logic errors, edge cases, and error handling gaps in the codebase.

## Scope

Reads: source code files. Focuses on whether the code does what it's supposed to do correctly.

## Process

### 1. Bug Detection

Scan for common bug categories:
- Off-by-one errors in loops and array access
- Null/undefined/nil dereference risks
- Race conditions in concurrent code
- Resource leaks (unclosed files, connections, streams)
- Integer overflow/underflow
- Incorrect boolean logic (De Morgan violations, short-circuit misuse)
- Incorrect comparisons (reference vs value equality)

### 2. Type Safety

- Proper use of types, generics, type guards (if applicable)
- Implicit type coercion risks
- Unchecked type assertions or casts
- Missing type annotations at boundaries

### 3. Error Handling

- Are errors caught, propagated, and reported properly?
- Swallowed exceptions (catch blocks that do nothing)
- Missing error handling for I/O, network, or external calls
- Inconsistent error propagation (some paths throw, others return null)
- Error messages that are unhelpful or leak internal details

### 4. Edge Cases

- Empty inputs (empty strings, empty arrays, null objects)
- Boundary values (0, -1, MAX_INT, empty collections)
- Concurrent access to shared state
- Partial failures in multi-step operations
- Unicode and encoding edge cases

### 5. Input Validation

- Validation at system boundaries (user input, API requests, file reads)
- Missing validation that could lead to crashes or incorrect behavior
- Validation that is present but insufficient (e.g., length check but no format check)

## Output Format

```markdown
# Correctness Review

## Critical Bugs
| Location | Bug | Impact | Fix |
|----------|-----|--------|-----|
| [file:line] | [description] | [what goes wrong] | [how to fix] |

## Error Handling Issues
| Location | Issue | Severity | Fix |
|----------|-------|----------|-----|
| [file:line] | [description] | critical/high/medium/low | [suggestion] |

## Edge Cases Not Handled
| Location | Missing Case | Risk | Fix |
|----------|-------------|------|-----|
| [file:line] | [scenario] | [what happens] | [suggestion] |

## Input Validation Gaps
[List of boundaries where validation is missing or insufficient]

## Recommendations
[Prioritized list — most impactful correctness fixes first]
```
