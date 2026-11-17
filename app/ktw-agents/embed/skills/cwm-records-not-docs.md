---
name: cwm-records-not-docs
description: >-
  The compactness invariant CWM enforces on every record write. Explains
  the per-storage-shape budgets, how to split oversized content, and
  when a record should become a document instead.
iconName: ruler
icon: "📏"
hashtags: [cwm, records, budget, compactness]
category: cwm
advertised: true
---

# Records, Not Documents

CWM's #1 invariant: every record object is a RECORD, not a document. Records fit in tens-of-lines of YAML; documents go in their own type with separate budget. The substrate enforces this at every write — there is no way to bypass.

## The budget classes

Three storage shapes, three default budgets (set per workspace in `workspace.yaml`, defaults shown):

| Shape | Default budget | Used for |
|-------|---------------|----------|
| `yaml-inline` | 4 KB | `rule`, `decision`, `goal`, `feature`, `approval-request` — single-record types |
| `ring-buffer` | 8 KB | Compact streams (e.g. recent journal compacts) |
| `yaml-with-body` | 64 KB | `document`, `source-document` — types that legitimately carry prose |

Each object type can override its budget via `max_size_bytes` in its type declaration. Use the override sparingly — the default budget is the contract.

## What happens when you exceed

```
cwm-object-create(module="<module>", object={type: "rule", name: "<rule>", body: <long prose>})
→ cwm: budget exceeded: kind="rule" size=8721 max=4096
```

The substrate refuses the write. The fix is **NEVER**:

- ✗ Raise the budget (`max_size_bytes: 16384`).
- ✗ Compress the prose into one object with shorter words.

The fix is one of:

1. **Split** — multiple objects, one concept each. A long decision document is N `decision` objects, one question each, its how in `rule` objects `justified-by` them.
2. **Reshape** — if the content is genuinely prose, the right type is `document` or `source-document`, not `rule`.
3. **Externalize** — long external content goes in `source-document` with a `source_uri` + content snapshot, not pasted inline.

## How to know if your record is too big

Rule of thumb: a record should be readable in 10-15 lines of YAML. If you've reached:

- More than 5 fields of free-form prose → too big.
- Multiple bullet lists per record → split into multiple objects.
- "I'll just paste the whole document here" → make a `document` instead.

## The briefing implication

A briefing carries short, typed values, not prose bodies. If your downstream agent needs the body, it issues `cwm-object-get(module, id)`. Big records are slow to fetch + slow to read; compact records win on every metric.

## Budget vs depth

An object can carry many child records (no cap on count). Compactness is per-record, not per-parent. A workspace with 200 decisions, each 600 bytes, is fine. One decision of 100 KB is not.

## What to read

- [cwm-knowledge-capture](cwm-knowledge-capture.md) — picking the right type for the content.
- [cwm-source-ingestion](cwm-source-ingestion.md) — how `source-document` handles long external prose.
