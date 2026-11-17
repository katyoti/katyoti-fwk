# Test Coverage Review

## Purpose

Evaluate the quality, completeness, and health of a project's test suite. Not just line coverage percentage — assess whether the right things are tested, how well, and whether the tests themselves are trustworthy.

## Scope

Reads: test files, source files, CI configuration, coverage reports (if present), test framework configuration. Focuses on what is tested, how it is tested, and what is missing.

## Process

### 1. Test Suite Inventory

Map what testing infrastructure exists:

- Test framework(s) in use (Jest, pytest, Go test, Vitest, JUnit, etc.)
- Test types present: unit, integration, end-to-end, contract, snapshot, property-based
- Coverage tooling configured (Istanbul, coverage.py, go cover, etc.)
- CI integration: are tests run on every commit/PR?
- Test file locations and naming conventions (collocated vs `__tests__` / `tests/` directory)

Produce a summary:

| Test Type | Present | Count (approx.) | Coverage Tool |
|-----------|---------|----------------|---------------|
| Unit | yes/no | [N] | [tool or none] |
| Integration | yes/no | [N] | — |
| E2E | yes/no | [N] | — |
| Contract | yes/no | [N] | — |

### 2. Critical Path Coverage

Identify the most important code paths (business logic, public API surface, error handling, authentication/authorization) and assess whether they have tests:

| Critical Path | Has Tests | Test Quality | Gap |
|--------------|-----------|-------------|-----|
| [feature/function] | yes/partial/no | strong/adequate/weak | [what's missing] |

Rules:
- Public API methods with no tests = High severity
- Business logic with no tests = High severity
- Authentication and authorization paths with no tests = Critical
- Error handling paths (catch blocks, error returns) with no tests = Medium

### 3. Test Quality

For tests that exist, evaluate their quality:

#### 3.1 Assertions
- Tests with no assertions (tests that always pass regardless of code behavior)
- Single-assertion tests for complex behavior that should verify multiple outcomes
- Assertions on implementation details rather than behavior (testing private methods, internal state)
- Overly broad assertions that will pass even when the code is wrong (`expect(result).toBeTruthy()`)

#### 3.2 Test isolation
- Tests that depend on execution order (passing alone but failing in sequence)
- Shared mutable state between tests (global variables, shared DB state without cleanup)
- Tests that depend on the current time, system locale, or environment variables without mocking
- Tests that make real network calls instead of using mocks/fixtures

#### 3.3 Test clarity
- Test names that don't describe what they verify ("test1", "it works", "happy path")
- Arrange/Act/Assert structure absent or unclear
- Giant test functions testing multiple unrelated behaviors (should be split)
- Copy-pasted test setup that should be a shared fixture or factory

#### 3.4 Flakiness signals
- `sleep()` / `setTimeout()` / `time.Sleep()` used to wait for async operations instead of proper awaiting
- Tests marked `.skip` or `xtest` without a tracking issue
- Tests with `TODO` or `FIXME` comments
- Retry logic inside tests masking real failures

### 4. Edge Case Coverage

For each critical function or module, check whether edge cases are tested:

- Empty inputs (empty string, empty array, null/nil/None)
- Boundary values (0, -1, MAX_INT, single-element collections)
- Error conditions (what happens when dependencies fail?)
- Concurrent access (if the code has concurrency, are race conditions tested?)
- Large inputs (does the code degrade gracefully at scale?)

### 5. Test-to-Code Ratio and Coverage Signals

Even without a coverage report, signal coverage health:

- Files or directories with zero test files
- Large source files (> 200 lines) with no corresponding tests
- Recently added features (check git blame / recent commits) with no tests
- If a coverage report is present: functions or branches at 0% coverage

### 6. Test Architecture

Evaluate the structural health of the test suite itself:

- **Test helpers and factories**: is there reusable infrastructure, or is setup duplicated everywhere?
- **Fixtures and test data**: managed centrally or scattered and inconsistent?
- **Mock strategy**: are mocks scoped correctly? Over-mocking (mocking everything including the thing being tested)? Under-mocking (tests hitting real databases in unit tests)?
- **Test database / environment isolation**: does each test start from a clean state?
- **Test build speed**: obviously slow test suites (thousands of E2E tests, no unit tests) that indicate a structural problem

### 7. Missing Test Types

Based on the project type, flag missing test layers:

| Project Type | Expected Test Types | Missing |
|-------------|-------------------|---------|
| REST API | unit + integration + contract | [what's absent] |
| CLI tool | unit + integration (subprocess) | [what's absent] |
| Library/SDK | unit + property-based | [what's absent] |
| Frontend app | unit + component + E2E | [what's absent] |
| Data pipeline | unit + integration + data validation | [what's absent] |

## Output Format

```markdown
# Test Coverage Review

## Test Suite Health
[Strong / Adequate / Needs Work / Critical Gap]

## Inventory
| Test Type | Present | Approx. Count | Notes |
|-----------|---------|--------------|-------|
| Unit | yes/no | [N] | [framework] |
| Integration | yes/no | [N] | — |
| E2E | yes/no | [N] | — |

## Critical Path Coverage
| Critical Path | Has Tests | Quality | Gap |
|--------------|-----------|---------|-----|
| [path] | yes/partial/no | strong/adequate/weak | [description] |

## Test Quality Issues
| Location | Issue | Type | Severity | Fix |
|----------|-------|------|----------|-----|
| [file:line] | [description] | no-assertions/flaky/isolation/clarity | critical/high/medium/low | [suggestion] |

## Edge Case Gaps
| Function/Module | Missing Edge Case | Risk | Fix |
|----------------|-----------------|------|-----|
| [file:function] | [scenario not tested] | [what could go wrong] | [test to add] |

## Coverage Signals
[Files, directories, or functions identified as likely untested]

## Test Architecture Issues
[Structural problems: missing factories, over-mocking, no isolation, etc.]

## Missing Test Types
[Test layers absent that the project type warrants]

## Recommendations
[Prioritized list — highest risk gaps first. Include suggested test names or scenarios where useful.]
```

## Severity definitions

- **Critical**: authentication/authorization path untested, core business logic with no tests, tests with no assertions (false confidence)
- **High**: public API surface untested, error handling paths untested, significant flakiness
- **Medium**: edge cases missing, test isolation problems, poor test naming
- **Low**: structural improvement (extract helper), minor clarity issue

## Rules

- Do not penalize a project for low line-coverage percentage alone — a small suite of high-quality tests targeting critical paths is better than 90% coverage with trivial assertions
- Flag false confidence (tests that always pass) as Critical — worse than no tests
- Every test gap flagged must include a concrete description of a test that should be added
- Do not suggest testing implementation details — focus on behavior and contracts
- If no tests exist at all, say so clearly and recommend the highest-value tests to write first
