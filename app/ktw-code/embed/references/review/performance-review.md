# Performance Review

## Purpose

Identify performance bottlenecks, inefficient algorithms, unnecessary allocations, and missed optimization opportunities in the codebase. Focuses on real-world impact — not micro-optimizations, but patterns that measurably hurt throughput, latency, or resource consumption under realistic load.

## Scope

Reads: source code, configuration files, dependency manifests, database schema or query definitions if present. Focuses on runtime performance, not build-time performance.

## Process

### 1. Algorithmic Complexity

Identify functions and loops with poor time or space complexity:

- O(n²) or worse where O(n log n) or O(n) is achievable — flag nested loops over data sets
- Repeated traversals of a collection that could be combined into one pass
- Linear searches in hot paths where a map/set lookup would be O(1)
- Sorting on every call where the input is already sorted or could be pre-sorted
- Recursive algorithms without memoization where results are recalculated
- Unbounded operations with no size guard (e.g., loading all rows from a table)

For each: state current complexity, achievable complexity, and the concrete fix.

### 2. Database & Query Performance

- **N+1 query patterns**: loop calling the database once per item instead of a single batch query
- **Missing indexes**: queries filtering or sorting on unindexed columns
- **SELECT \***: fetching all columns when only a subset is needed
- **Missing pagination**: queries that return unbounded result sets
- **Queries inside transactions**: long-running transactions holding locks
- **ORM pitfalls**: lazy loading in loops, missing `.include()`/`.preload()` / eager loading
- **Missing query result caching**: the same query executed repeatedly with identical parameters in the same request lifecycle

### 3. Caching

- Results that are expensive to compute and rarely change but are recomputed on every request
- Missing HTTP cache headers for static or slowly-changing responses
- Missing memoization for pure functions called repeatedly with the same arguments
- Cache invalidation bugs: stale data served after mutations
- Over-caching: caching data that changes frequently, causing stale reads
- Cache stampede risk: no locking or staggering on cache misses for expensive computations

### 4. I/O and Concurrency

- **Blocking I/O on hot paths**: synchronous file reads, synchronous network calls in request handlers
- **Sequential I/O that could be parallel**: multiple independent fetches done one-by-one instead of concurrently (`Promise.all`, goroutines, async gather, etc.)
- **Missing connection pooling**: creating new DB or HTTP connections per request
- **Unbuffered I/O**: writing byte-by-byte to streams that should be buffered
- **Missing streaming**: loading entire files or responses into memory when streaming would suffice
- **Thread/goroutine/coroutine leaks**: spawned workers that are never joined or cancelled

### 5. Memory

- **Large object allocations in tight loops**: creating objects that could be pooled or reused
- **Accumulation without bounds**: growing lists/maps with no eviction or size cap
- **String concatenation in loops**: building strings by concatenation instead of a builder
- **Unnecessary deep copies**: cloning large objects where a reference suffices
- **Retaining references in closures**: preventing garbage collection of large objects
- **Byte slice / buffer reuse**: buffers allocated fresh for each operation that could be pooled

### 6. Hot Path Analysis

Identify the critical paths — the code that runs on every request, every event, or every iteration — and scrutinize them more carefully:

- Any logging at DEBUG or INFO level inside tight loops (even if log level is off, the string formatting may still occur)
- Reflection or dynamic dispatch inside hot loops
- Regex compilation inside loops (compile once, reuse)
- JSON marshalling/unmarshalling of the same structure repeatedly when it could be cached
- Mutex/lock contention in hot paths

### 7. Frontend Performance (if applicable)

For web front-ends:

- Render-blocking resources (scripts and styles in `<head>` without `async`/`defer`)
- Large bundle sizes — missing code splitting, dead code in bundles
- Missing lazy loading for images and off-screen components
- Expensive re-renders: components re-rendering on every parent state change without memoization
- Missing virtualization for long lists
- Synchronous localStorage / sessionStorage access in render paths

## Output Format

```markdown
# Performance Review

## Performance Score
[Low Risk / Medium Risk / High Risk] — [one-line summary of dominant issue]

## Algorithmic Complexity
| Location | Current Complexity | Achievable | Impact | Fix |
|----------|--------------------|-----------|--------|-----|
| [file:line] | O(n²) | O(n) | [description] | [concrete suggestion] |

## Database & Query Issues
| Location | Issue | Type | Severity | Fix |
|----------|-------|------|----------|-----|
| [file:line] | [description] | N+1/missing-index/unbounded/etc. | critical/high/medium/low | [suggestion] |

## Caching Opportunities
| Location | What to Cache | Rationale | Suggested Approach |
|----------|--------------|-----------|-------------------|
| [file:line] | [what] | [why expensive / stable] | [memoize / HTTP cache / Redis / etc.] |

## I/O & Concurrency Issues
| Location | Issue | Impact | Fix |
|----------|-------|--------|-----|
| [file:line] | [description] | [latency / throughput impact] | [suggestion] |

## Memory Issues
| Location | Issue | Risk | Fix |
|----------|-------|------|-----|
| [file:line] | [description] | [leak / GC pressure / OOM] | [suggestion] |

## Hot Path Issues
| Location | Issue | Fix |
|----------|-------|-----|
| [file:line] | [description] | [suggestion] |

## Frontend Performance (if applicable)
[Findings and suggestions]

## Recommendations
[Prioritized list — highest real-world impact first. Include estimated impact where possible (e.g., "eliminates N+1 on the users endpoint — likely 10x latency reduction under load").]
```

## Severity definitions

- **Critical**: likely to cause timeouts, OOM errors, or complete failure under realistic load
- **High**: measurable degradation at moderate load (> 100 concurrent users or > 10k records)
- **Medium**: noticeable in profiling or at scale, but not immediately user-facing
- **Low**: micro-optimization or theoretical concern with negligible real-world impact

## Rules

- Do not flag micro-optimizations (single nanosecond gains) unless they are in a hot path
- Always state the realistic load scenario that would trigger the issue
- Never suggest premature optimization — if the code is fast enough for its context, say so
- For every issue flagged, provide a concrete code-level fix, not just a category label
- If profiling data is available (flamegraphs, benchmarks, APM traces), reference it
