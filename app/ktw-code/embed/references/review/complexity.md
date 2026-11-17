# Complexity Review (Einstein Analysis)

## Purpose

Evaluate whether the code is at the right level of complexity: "Everything should be made as simple as possible, but not simpler."

## Scope

Reads: source code and documentation. Focuses on identifying both over-engineering and under-engineering.

## Process

### 1. Over-Complicated Code

Identify code that is more complex than necessary:

- **Premature abstractions**: interfaces/classes with a single implementation, abstraction layers with no variation
- **Over-engineered solutions**: Enterprise FizzBuzz syndrome — complex machinery for simple problems
- **Premature generalization**: code built for hypothetical future requirements that may never arrive
- **Deep inheritance hierarchies**: where composition would be simpler
- **Excessive patterns**: design patterns used where straightforward code would suffice
- **Configuration-driven logic**: where a simple `if` statement would be clearer
- **Meta-programming / reflection**: used where static code would work
- **Unnecessary indirection**: wrappers, adapters, facades that add no logic
- **Over-parameterized functions**: too many parameters or options for the actual use cases

### 2. Under-Complicated Code

Identify code that is too simplistic for the problem:

- **Missing abstractions**: long procedural blocks that should be decomposed
- **Hardcoded values**: that should be configurable
- **No separation of concerns**: business logic mixed with I/O, UI, or infrastructure
- **Missing error handling**: for a domain that requires it
- **Inline SQL/HTML/etc.**: embedded in application code without separation
- **No dependency injection**: where testability and flexibility demand it
- **Primitive obsession**: using strings/ints where domain types would be safer
- **No structure**: where complexity warrants modules, classes, or patterns

### 3. Over-Complicated Documentation

- Redundant sections covering the same topic
- Overly complex structure for simple content
- Unnecessary jargon where plain language suffices
- Circular or self-referencing explanations

### 4. Classify Each Issue

For each issue found:
- **Accidental complexity**: can be removed without losing functionality
- **Essential complexity**: inherent to the problem, but could be better managed

## Output Format

```markdown
# Complexity Review

## Complexity Score
[Low / Medium / High]

## Over-Complicated
| Location | What | Why It's Too Complex | Simplification |
|----------|------|---------------------|----------------|
| [file:line] | [description] | [what's unnecessary] | [concrete suggestion] |

## Under-Complicated
| Location | What | Why It's Too Simple | What's Needed |
|----------|------|--------------------|--------------|
| [file:line] | [description] | [what's missing] | [concrete suggestion] |

## Classification
| Issue | Type | Impact |
|-------|------|--------|
| [issue] | Accidental / Essential | [what we gain by fixing] |

## Trade-offs
[For each proposed simplification: what we lose vs what we gain]

## Verdict
[Overall complexity assessment — is the code at the right level for its problem?]
```
