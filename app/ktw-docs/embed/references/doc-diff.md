# Document Diff Analysis

Reference for comparing two versions of a document or two competing documents.

---

## Purpose

Compare two documents to identify what changed, what was added, what was removed, and whether the changes are internally consistent. Produces a structured diff report with semantic analysis — not just a line-by-line diff, but an assessment of meaning, intent, and impact of the changes.

## Scope

Input: two documents (versions of the same document, or two competing specs/proposals).
Allowed tools: Read, Grep, Glob.
Output: Markdown report in the same language as the input documents.

argument-hint: [file-path-A] [file-path-B]

The two documents may be provided as:
- Two file paths
- Two pasted blocks of text (labeled A/B or v1/v2 or Old/New)
- A file path and a pasted block

If only one document is given, ask the user to provide the second before proceeding.

## Process

### Step 1 — Load and classify both documents

1. Read both documents in full.
2. Determine the relationship: version evolution (v1 → v2) or competing alternatives (option A vs option B).
3. Identify the document type (spec, requirements, architecture, policy, contract, prose, etc.).
4. Note the apparent purpose of each document.

### Step 2 — Structural diff

Map the structure of each document:
- List all top-level and second-level sections for each.
- Identify: sections present in A but not B, sections present in B but not A, sections present in both.

### Step 3 — Run all analysis categories

#### Category A — Content changes

- A1 -- Additions: Content present in B but not in A. For each: what was added, in which section, and what is the likely intent.
- A2 -- Removals: Content present in A but not in B. For each: what was removed, in which section, and whether the removal appears intentional or accidental.
- A3 -- Modifications: Content that exists in both but was changed. For each: what changed, what the change means semantically (not just textually).
- A4 -- Moved content: Content that appears in a different section in B than in A. Distinguish moves from rewritten duplicates.

#### Category B — Semantic and intent analysis

- B1 -- Scope changes: Did the document's scope expand, contract, or shift? Cite evidence.
- B2 -- Requirement / obligation changes: For specs and requirements docs — were any obligations weakened (shall → should), strengthened (should → shall), added, or removed?
- B3 -- Terminology shifts: Are the same concepts now called different things? Is this a deliberate rename or an inconsistency?
- B4 -- Tone and stance changes: Did the document become more prescriptive, more permissive, more formal, more hedged? Characterize the shift.
- B5 -- Decision changes: Were any explicit decisions reversed, softened, or newly introduced?

#### Category C — Consistency of changes

- C1 -- Internal consistency of B: Do the changes in B cohere with each other? Or do some additions contradict other additions?
- C2 -- Orphaned references: Did A reference a section or document that was removed in B, leaving a dangling reference?
- C3 -- Incomplete propagation: Was a term renamed in one place but not all places? Was a rule changed in one section but a related section still reflects the old rule?
- C4 -- Version / date / metadata update: If B is a new version, were the version number, date, and changelog/revision history updated?

#### Category D — Impact assessment

- D1 -- Breaking changes: Changes that would require existing implementations, integrations, or processes to be updated. Flag these explicitly.
- D2 -- Risky removals: Content removed from A that may be needed — missing error handling, missing constraints, deleted acceptance criteria.
- D3 -- Improvement assessment: Which changes in B are clear improvements over A? Which are regressions or neutral?
- D4 -- Open questions raised by changes: Changes that introduce new ambiguity or that appear to need follow-up decisions.

### Step 4 — Produce the report

Write the report in the same language as the analyzed documents (use the majority language if they differ).

Follow this template strictly:

```
# Document Diff Analysis

Analysis date: {today's date}
Document A: {file name or "Pasted block A"}
Document B: {file name or "Pasted block B"}
Relationship: {Version evolution (A → B) | Competing alternatives}
Document type: {detected type}

---

## Summary

{3-5 sentences: nature and scale of changes, most significant additions/removals, overall assessment of whether B is an improvement, any critical issues introduced.}

---

## Structural overview

### Sections in A only (removed in B)
{List. If none: "None."}

### Sections in B only (added in B)
{List. If none: "None."}

### Sections in both
{List.}

---

## A — Content changes

### A1 -- Additions
{For each addition: section, summary of what was added, likely intent.}

### A2 -- Removals
{For each removal: section, summary of what was removed, assessment of whether intentional.}

### A3 -- Modifications
{For each modification: what changed and what it means semantically.}

### A4 -- Moved content
{Content relocated between sections, with from/to.}

---

## B — Semantic and intent analysis

### B1 -- Scope changes
{Scope expansion, contraction, or shift with evidence.}

### B2 -- Requirement / obligation changes
{Strengthened, weakened, added, or removed obligations.}

### B3 -- Terminology shifts
{Renamed concepts — intentional or inconsistency.}

### B4 -- Tone and stance changes
{How the register or stance shifted.}

### B5 -- Decision changes
{Reversed, softened, or newly introduced decisions.}

---

## C — Consistency of changes

### C1 -- Internal consistency of B
{Whether the changes in B cohere with each other.}

### C2 -- Orphaned references
{Dangling references created by removals.}

### C3 -- Incomplete propagation
{Rename or rule change applied in some places but not all.}

### C4 -- Version / date / metadata update
{Whether version info was updated in B.}

---

## D — Impact assessment

### D1 -- Breaking changes
{Changes requiring existing implementations or processes to update.}

### D2 -- Risky removals
{Removed content that may be needed.}

### D3 -- Improvement assessment
| Change | Assessment | Notes |
|--------|-----------|-------|
| {description} | Improvement / Regression / Neutral | {reason} |

### D4 -- Open questions raised by changes
{New ambiguities or decisions left unresolved by the changes.}

---

## Change summary table

| # | Category | Change | Type | Severity |
|---|----------|--------|------|----------|
| 1 | {A1..D4} | {Short description} | {Added/Removed/Modified/Moved} | {High/Medium/Low} |

---

## Priority recommendations

{Numbered list of the most important actions — things to fix in B, decisions to make, follow-up needed. Ranked by decreasing impact.}
```

## Severity definitions

- High: Breaking change, risky removal of a constraint or requirement, internal inconsistency in B, orphaned reference.
- Medium: Incomplete propagation of a rename or rule change, obligation weakened without explanation, open decision left unresolved.
- Low: Metadata not updated, neutral terminology shift, cosmetic restructuring.

## Report rules

- Always cite specific sections or line numbers when possible.
- For every High or Medium issue, propose a concrete fix.
- For version evolution (A → B): frame findings as "what changed and whether it's correct."
- For competing alternatives (A vs B): frame findings neutrally — do not recommend one over the other unless a clear factual advantage exists.
- Do not flag content that is legitimately different between versions as an issue — only flag problems.
