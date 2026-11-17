# Example — E1 minimal ISA

A fictitious teaching example ("ColorlessCLI"). The smallest valid ISA:
Goal + Criteria only. Everything below the rule is the ISA file verbatim.

---

```markdown
---
subject: "Add a --no-color flag to a CLI tool"
depth: E1
phase: build
progress: 0/4
updated: 2026-04-28
---

## Goal

Add a `--no-color` flag to the `dump.ts` CLI so output strips ANSI escape
codes when the flag is present (or when `NO_COLOR` env is set, per the
no-color.org convention).

## Criteria

- [ ] ISC-1: `dump.ts --no-color` output contains zero ANSI escape sequences (probe: `dump.ts --no-color | rg -c $'\x1b\['` returns 0).
- [ ] ISC-2: the env-var path also strips (probe: `NO_COLOR=1 dump.ts | rg -c $'\x1b\['` returns 0).
- [ ] ISC-3: default invocation still emits color when stdout is a TTY (probe: `script -q /dev/null dump.ts | rg -c $'\x1b\['` returns ≥1).
- [ ] ISC-4: Anti: `--no-color` emits nothing new to stderr (probe: `dump.ts --no-color 2>&1 >/dev/null | wc -c` returns 0).
```

---

Why this is a good E1: the Goal sentence plus four binary probes carries the
entire articulation — Problem/Vision/etc. are omitted because the depth gate
does not require them and they would add nothing. The `Anti:` criterion
(ISC-4) is what keeps a sloppy implementation from passing: adding a "color
disabled" log line would technically meet ISC-1 while breaking the
silent-by-default expectation.
