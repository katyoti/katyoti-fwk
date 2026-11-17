# Example — E3 project ISA

A fictitious teaching example ("ArxivExtractor"). A mid-size project with
the eight E3-required sections populated. Everything below the rule is the
ISA file verbatim. Note the soft floor: 12 ISCs is under the E3 floor of 32
— acceptable here because the under-count is deliberate and the work is a
single-file CLI; a real E3 would record that justification in Decisions.

---

```markdown
---
subject: "CLI extracting arxiv paper metadata into JSONL"
depth: E3
phase: build
progress: 5/12
updated: 2026-02-12
---

## Problem

Researching across arxiv papers means reading abstracts in a browser one at
a time. There is no quick "give me title, authors, abstract, categories,
and submission date for these 50 paper IDs as JSONL" tool. The arxiv API
exists but its XML response shape is annoying enough that nobody uses it
casually.

## Vision

A single bun TypeScript CLI: `bun arxiv.ts <id1> <id2> ... > papers.jsonl`.
One paper per line, structured fields, no friction. Euphoric surprise:
feeding 100 IDs and getting clean JSONL back in under three seconds.

## Out of Scope

- No PDF download. Metadata only.
- No citation graph traversal. Single-paper lookup, no following references.
- No web UI. CLI exclusively.
- No persistent cache. Stateless; every run hits the API.

## Constraints

- Bun runtime only. No Node dependency.
- Zero npm dependencies — built-in `fetch` and a hand-rolled XML parse.
- Must respect arxiv's API rate limits (3 requests/second per their TOS).

## Goal

Ship a single-file `arxiv.ts` CLI that takes paper IDs as arguments, queries
the arxiv Atom API, parses the response, and writes one JSONL row per paper
to stdout with fields: `id`, `title`, `authors`, `abstract`, `categories`,
`submitted`, `updated`.

## Criteria

- [x] ISC-1: `arxiv.ts` is a single file at the project root.
- [x] ISC-2: zero entries in `package.json` dependencies (probe: `jq '.dependencies | length' package.json` returns 0).
- [ ] ISC-3: `bun arxiv.ts 2401.12345` returns exactly one JSONL row to stdout.
- [ ] ISC-4: the JSONL row has exactly seven fields: `id, title, authors, abstract, categories, submitted, updated`.
- [x] ISC-5: `authors` is an array of strings, never a concatenated string.
- [ ] ISC-6: `categories` is an array of strings (e.g. `["cs.AI", "cs.LG"]`).
- [ ] ISC-7: a 100-ID batch completes in ≤3s wall clock (rate-limit-aware throttling).
- [x] ISC-8: a bad ID (e.g. `9999.99999`) writes a JSONL row with an `error` field instead of crashing.
- [ ] ISC-9: stderr stays empty on a successful 100-ID run.
- [ ] ISC-10: `bun arxiv.ts --help` prints usage in ≤12 lines.
- [ ] ISC-11: Anti: `--download` is not a recognized flag (returns help + exits 2).
- [x] ISC-12: Anti: never more than 3 concurrent requests against the arxiv API.

## Test Strategy

- isc: ISC-3 | type: cli-probe | check: stdout has exactly one JSONL row | threshold: 1 line | tool: `bun arxiv.ts 2401.12345 | wc -l`
- isc: ISC-7 | type: performance | check: wall-clock for 100 IDs | threshold: ≤3000ms | tool: `time bun arxiv.ts $(cat 100-ids.txt)`
- isc: ISC-8 | type: error-handling | check: bad ID does not exit non-zero | threshold: exit 0 + error field | tool: `bun arxiv.ts 9999.99999 | jq -e '.error'`
- isc: ISC-12 | type: anti-probe | check: max concurrent requests | threshold: ≤3 | tool: instrument fetch with a counter

## Features

- name: AtomFetch | satisfies: [ISC-7, ISC-12] | depends_on: [] | parallelizable: false
- name: AtomParse | satisfies: [ISC-4, ISC-5, ISC-6] | depends_on: [AtomFetch] | parallelizable: false
- name: CLIInterface | satisfies: [ISC-3, ISC-8, ISC-9, ISC-10, ISC-11] | depends_on: [AtomParse] | parallelizable: false

## Decisions

- 2026-02-01 18:00: Hand-rolled XML parse over a library — Bun has no built-in XML, the response shape is bounded, and a dep would violate the zero-deps constraint.
- 2026-02-08 22:30: ❌ DEAD END: Promise.all() with 100 IDs — arxiv rate-limited after request 12. Reverted to a 3-concurrency queue. Don't retry.
```
