# Performance Budgets

Use these budgets as baselines when evaluating runs. A run that exceeds a budget gets a ⚠ or ✗ tag depending on the overshoot.

## Per-Task Budgets

| Task type | Max turns | Max tokens | Max duration | Notes |
|-----------|-----------|------------|-------------|-------|
| Simple (hello-world) | 10 | 20,000 | 30s | 3-5 turns expected |
| Analysis (spec-extract) | 30 | 100,000 | 120s | 10-20 turns typical |
| Planning (goal-planner) | 50 | 200,000 | 300s | Multi-delegation expected |
| Creative (song-writer) | 15 | 50,000 | 60s | Few tools, mostly text |

## Per-Agent Budgets (within delegation)

| Mode | Max turns | Max tokens | Notes |
|------|-----------|------------|-------|
| worker | 15 | 50,000 | Inline — parent waits |
| background | 25 | 100,000 | Async — more freedom |
| coordinator | 10 | 30,000 | Should delegate, not do work |

## Tool Budgets

| Pattern | Threshold | Tag if exceeded |
|---------|-----------|-----------------|
| Same tool called 3+ times with same args | Any | ✗ wasteful — likely retry loop |
| file-write without prior file-read on same path | 1 occurrence | ⚠ acceptable (first write) |
| >5 glob/grep calls without narrowing | 5 calls | ⚠ acceptable — consider better prompt |
| bash with exit code != 0, retried >2 times | 2 retries | ✗ wasteful — should adapt |
| LLM turn with no tool calls and no text output | 1 occurrence | ✗✗ failed — empty turn |

## Efficiency Thresholds

| Metric | Good (✓) | Acceptable (⚠) | Wasteful (✗) | Failed (✗✗) |
|--------|----------|-----------------|--------------|-------------|
| Turns vs budget | <50% | 50-100% | 100-150% | >150% |
| Tokens vs budget | <60% | 60-100% | 100-200% | >200% |
| Error rate | 0% | <10% | 10-30% | >30% |
| Retry rate | 0 retries | 1-2 retries | 3-5 retries | >5 retries |

## How to Apply

1. Look up the task type → get the budget
2. Compare actual metrics vs budget
3. For each metric, assign a tag using the thresholds
4. The overall efficiency tag = worst individual tag (conservative)
5. Record evidence: "Turns: 8/10 budget (✓), Tokens: 45000/50000 (⚠ — close to limit)"
