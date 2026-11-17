# Library / Package Audit

## Purpose

Evaluate a reusable library, SDK, framework, or package for API quality, extensibility, and completeness.

## Applicability

Only run this review when the target is a library, SDK, framework, or reusable package — not application code. Skip for standard applications, services, or scripts.

## Scope

Reads: public API surface, exports, types, documentation, configuration options, extension points.

## Process

### 1. API Consistency

- Are naming conventions uniform across the public API (functions, classes, methods, parameters, return types)?
- Are similar operations handled with consistent parameter order, error reporting, and return value shape?
- Flag inconsistencies: some methods return promises while similar ones use callbacks, or mixed naming conventions (camelCase vs snake_case)
- Are error types/codes consistent across the API?

### 2. Extensibility

- Can the library be extended without modifying its source?
- Identify extension mechanisms: plugins, hooks, middleware, subclassing, configuration, event emitters
- If extensibility is lacking, recommend specific mechanisms to add
- Flag sealed/final designs that prevent reasonable extension points
- Assess whether the extension points are documented

### 3. Completeness

- For this type of package, what functionalities are standard and expected?
- Identify missing features users would reasonably expect (e.g., a date library without timezone support, an HTTP client without retry/timeout options)
- Flag placeholder or stub implementations that are not yet functional
- Check if common use cases are covered by the API

### 4. API Surface Quality

- Is the API surface too large (too many exports, hard to learn)?
- Is the API surface too small (common tasks require workarounds)?
- Are defaults sensible (work out of the box for common cases)?
- Is the learning curve appropriate for the target audience?

### 5. Type Safety

- Are types exported and accurate?
- Do generics provide appropriate flexibility?
- Are edge cases typed correctly (null, undefined, empty)?

## Output Format

```markdown
# Library / Package Audit

## API Consistency
| Issue | Location | Expected | Actual | Fix |
|-------|----------|----------|--------|-----|
| [inconsistency] | [function/method] | [expected pattern] | [what's different] | [suggestion] |

## Extensibility
- **Extension mechanisms present**: [list]
- **Extension mechanisms missing**: [what's needed and why]
- **Sealed designs**: [areas that should be open for extension]

## Completeness
| Expected Feature | Status | Notes |
|-----------------|--------|-------|
| [feature] | present/missing/partial | [details] |

## API Surface Assessment
- **Size**: [appropriate / too large / too small]
- **Defaults**: [sensible / incomplete / surprising]
- **Learning curve**: [appropriate / steep / too shallow]

## Recommendations
[Prioritized list — most impactful improvements for package consumers]
```
