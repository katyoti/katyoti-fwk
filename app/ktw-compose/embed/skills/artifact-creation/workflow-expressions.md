---
name: workflow-expressions
description: "Expression-language (expr-lang) reference for ktw-agents workflows: scope variables, conditions, arithmetic, and collections. Load when writing or debugging string bindings like condition or items."
iconName: function-square
icon: "🧮"
hashtags: [workflow, expr-lang, expressions, bindings]
category: orchestration
advertised: false
---

# Expression Language (expr-lang)

The workflow engine uses [expr-lang](https://expr-lang.org/) for evaluating
string values in `input:`, `condition:`, `value:`, `target:`, `items:`, and
`on:` fields. Expressions are compiled once and cached per workflow
machine.

## Available variables (Scope)

```
inputs.<key>        ← workflow's own input map (TaskRunRequest.Input)
state.<key>         ← shared workflow data namespace (mutated via assign)
results.<step>      ← outputs of completed sibling steps (immutable)
loop.index          ← inside foreach: 0-based iteration counter
loop.item           ← inside foreach: current collection item
now                 ← current time (Go time.Time, injectable for testing)
```

## Common patterns

### Field access

```yaml
input:
  full: "results.fetch"               # entire output
  one_field: "results.fetch.title"    # field access (works on map / struct)
  nested: "results.fetch.user.name"   # multiple levels
```

### Boolean conditions

```yaml
- kind: if
  condition: "results.check.passed == true"

- kind: while
  condition: "state.retries < 3"
  max_iterations: 5

- kind: if
  condition: "len(results.list.items) > 0"   # built-in len() function
```

### Arithmetic + concatenation

```yaml
- kind: assign
  target: "retries"
  value: "state.retries + 1"

- kind: assign
  target: "summary"
  value: "results.a.text + ' | ' + results.b.text"
```

### Collections (for foreach)

```yaml
- kind: foreach
  items: "results.list-files.paths"

- kind: foreach
  items: "[1, 2, 3, 4, 5]"           # inline literal
```

### Conditional defaults

```yaml
input:
  workspace: "inputs.workspace ?? '/tmp/default'"
```

## Expression vs literal — what gets evaluated?

The workflow loader passes string-valued fields through `cache.Eval(...)`. If
evaluation succeeds, the result replaces the string. If evaluation FAILS
(parse error, unbound variable, type error), the original string is kept
as-is — meaning:

- A string that LOOKS like a literal (`"hello"`) IS one if it doesn't parse
  as an expression
- A string that LOOKS like an expression but should be literal needs to be
  formatted such that expr-lang rejects it

In practice this rarely bites because the prefixes (`inputs.`, `state.`,
`results.`, `loop.`) are explicit and distinctive. But beware of bare
identifiers — `"foo"` parses as a string literal `foo` (NOT an unbound
variable error), so it stays as the string `"foo"`.

## Evaluation context

Expressions evaluate against the **current** scope at the moment the
binding is resolved:

- For `input:` on a spawn step: resolved RIGHT BEFORE the child is spawned.
  All previous siblings' results are available.
- For `condition:` on an if/while: resolved before each potential branch /
  iteration.
- For `value:` on a return / assign: resolved at terminal/assign time.
- For `items:` on foreach: resolved once at the foreach's start (the
  iteration list is frozen).
- For `on:` on switch: resolved once when the switch is entered.

## Built-in functions

expr-lang ships with standard functions: `len`, `int`, `string`, `float`,
`type`, `keys`, `values`, etc. See https://expr-lang.org/docs/language-definition
for the full set. Custom functions can be added via the workflow factory
but are not configured by default.

## Pitfalls

1. **Type errors are silent fallbacks** — a failed eval returns the string
   as-is. If your binding seems wrong, double-check the field path against
   the actual results shape (use `--dry-trace` to inspect events).
2. **`results.<step>` only after step completes** — bindings can reference
   any earlier step's results, never future ones. Inside parallel branches,
   sibling results are NOT available (siblings run concurrently).
3. **`state.*` is mutable, `results.*` is not** — use `assign` to update
   state. Don't try to write to `results.X` (it's the spawner's output map).
4. **Always-false `condition` from typo** — a typo like
   `results.checkk.passed` evaluates to `nil`, which is falsy, silently
   dropping you into the `else` branch. Use `--dry-trace` to verify.
5. **`now` is for testing** — don't use `now` in production conditions
   unless you understand the scope timing. Better to inject a real time via
   `state` from a wait or external event.
