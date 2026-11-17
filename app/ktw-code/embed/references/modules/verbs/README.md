# Per-Verb References — code-module skill

Each `code-module` verb has its own reference file. Load them on demand — the parent skill ([`/ktw-code/skills/code-module.md`](../../../skills/code-module.md)) is intentionally lean; verb-level detail lives here.

## Verbs

| Verb | Reference | Purpose |
|---|---|---|
| `charter` | [`charter.md`](charter.md) | Bootstrap or refresh a module's charter |
| `status` | [`status.md`](status.md) | Print where one module stands |
| `lint` | [`lint.md`](lint.md) | Run rules + alignment checks |
| `plan-tier` | [`plan-tier.md`](plan-tier.md) | Compute tier-N gaps; create tasks |
| `verify` | [`verify.md`](verify.md) | UC-driven E2E with green/red output |
| `audit` | [`audit.md`](audit.md) | Run a fresh 13-agent code review |
| `promote` | [`promote.md`](promote.md) | Move module up a tier (gated) |
| `factory-status` | [`factory-status.md`](factory-status.md) | Portfolio rollup across all modules |

## Loading via the skill mechanism

```bash
ktw-code llm resources read ktw-code:/reference/modules/verbs/<verb>.md
```

E.g.:

```bash
ktw-code llm resources read ktw-code:/reference/modules/verbs/plan-tier.md
```

The skill itself ([`code-module.md`](../../../skills/code-module.md)) lists each verb's purpose in one line and points to the per-verb reference for full detail.

## Verb anatomy (consistent across all 8)

Every verb reference follows the same shape:

1. **Title + one-line purpose**
2. **Inputs** — flags and arguments
3. **When to use** — situational guidance
4. **Preconditions** — what must be true before the verb runs (only for verbs that have them)
5. **Behavior** — pseudocode of the verb's execution
6. **Output** — what artifacts the verb writes
7. **Output examples** — text / JSON / markdown samples (where applicable)
8. **Failure modes** — what causes the verb to refuse / error
9. **See also** — cross-references to other verbs and method docs

This consistency makes the verbs scannable: a reader knows where to find the inputs, what gets written, and what to do next.
