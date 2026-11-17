# Templates — code-module method

Reusable templates the skill consumes (or directs the user to copy).

## Files

| Template | Purpose | Used by |
|---|---|---|
| [`template.tasktest`](template.tasktest) | Reference template for L2 scenarios | `charter` verb (copies into `/modules/<module>/tests/L2/`); test authors (copy + edit per UC) |

## Loading

```bash
ktw-code llm resources read ktw-code:/reference/modules/templates/template.tasktest
```

## Adding a template

When a new reusable artifact emerges (e.g. a charter sub-section, a fixture format, a results-JSON schema), drop it here. Update this README's table.

Templates are **immutable** once referenced by tasks. To evolve, version (`_template-v2.tasktest`); old tasks pin to the old version.
