# Documentation Corpus Analysis

Reference for structural and consistency analysis of a documentation corpus.

---

## Purpose

Perform a comprehensive structural and consistency analysis of a collection of Markdown documents. Identify problems in organization, naming, terminology, completeness, cross-references, duplicates, and coverage.

## Scope

Input: a folder path or glob pattern pointing to a collection of Markdown documents.
Allowed tools: Read, Grep, Glob.
argument-hint: [folder-or-glob-pattern]

## Process

### Step 1 — Inventory

1. Glob the target path to collect all `.md` files.
2. Read every document to build a full inventory:
   - File name and path
   - Top-level title (`# ...`)
   - All section headings (with depth level)
   - All references to other documents (file names mentioned in text, links, backtick references)
   - Version and date footer if present
   - Approximate size (line count)
3. Build a document map: group documents by subfolder and by apparent type (main doc, annex, overview, etc.).

### Step 2 — Run all analysis categories

Execute every category and sub-check below. Never skip one. If no issue is found, state it explicitly.

#### Category A — Structure and naming

- A1 -- File naming convention: Do all files follow a consistent naming pattern? Identify the pattern(s) in use and flag outliers. Check for: consistent casing, consistent separators, consistent numbering scheme, consistent prefix/suffix.
- A2 -- Numbering gaps: Are there gaps in numbered sequences? (e.g., Annexe 01, 02, 04 — missing 03). Check both file names and section numbers within documents.
- A3 -- Section heading consistency: Do all documents use the same Markdown heading levels and style? (e.g., all start with `# Title`, then `## 1. Section`). Flag documents that deviate from the dominant pattern.
- A4 -- Mandatory sections: Identify sections that appear in most documents (e.g., Overview, References, Version footer, Validation). Flag documents missing these expected sections.
- A5 -- Section ordering: For documents of the same type, is the section order consistent? Flag reordered sections.

#### Category B — Content quality

- B1 -- Duplicate content: Detect paragraphs, sections, or tables that appear verbatim or near-verbatim in multiple documents. Flag copy-paste with exact locations.
- B2 -- Terminology consistency: Same concept called different names across documents? Build a terminology table of key terms and flag synonyms or inconsistencies (e.g., "manifest" vs "manifeste", "bundle" vs "lot", "flux" vs "flow"). Also flag inconsistent capitalization or spelling of the same term across the corpus (e.g., "API" vs "api", "back-end" vs "backend").
- B3 -- Version and date consistency: Are all documents at the same version? Are dates consistent and up-to-date? Flag stale or placeholder dates.
- B4 -- Placeholder detection: Scan entire corpus for `XX`, `TODO`, `TBD`, `TBC`, `A DEFINIR`, `a completer`, `...` used as placeholder, `N/A` used where a value is expected.

#### Category C — Cross-references

- C1 -- Broken references: Document A references document B, but B does not exist (file name mismatch, typo, deleted file).
- C2 -- Orphan documents: Documents not referenced by any other document in the corpus. Potentially disconnected or forgotten.
- C3 -- Circular reference chains: Detect A -> B -> C -> A loops. Note: simple bidirectional references (A <-> B) are acceptable; flag deeper chains (3+ documents).
- C4 -- Reference depth violation: References should point to documents by name only (e.g., `Annexe 03 - Format manifest.json.md`). Flag any reference that points to a specific section within another document (e.g., "see Annexe 03, section 5.2" or "see Annexe 03#section-5"). Only document-level references are allowed.
- C5 -- Reference format consistency: Are references formatted the same way across the corpus? (backtick `filename.md`, markdown link `[text](file.md)`, plain text). Identify the dominant pattern and flag deviations.

#### Category D — Coverage and completeness

- D1 -- Topic coverage matrix: Build a matrix showing what major topics/concepts each document covers. Identify gaps where an expected topic has no dedicated document or section.
- D2 -- Depth imbalance: Compare document sizes (line count). Flag significant outliers (a document 5x shorter or longer than its peers of the same type). This may indicate insufficient coverage or scope creep.
- D3 -- Missing counterparts: If a document describes one direction of a process (e.g., "Application -> File"), check that the opposite direction exists (e.g., "File -> Application"). Flag missing symmetric documents.
- D4 -- Root document / table of contents: Is there a root document or index that serves as table of contents and lists all documents? Does it reference all existing documents? Are any documents missing from the index?

#### Category E — Architecture

- E1 -- Responsibility boundaries: Is it clear which document owns which topic? Flag overlapping responsibilities where two documents both define rules for the same concept (different from duplicate content — this is about ownership ambiguity).
- E2 -- Layering: Are documents organized in clear layers? Expected: overview -> detailed docs -> annexes/appendices. Flag documents that mix abstraction levels.
- E3 -- Audience consistency: Are all documents targeting the same audience (developer, architect, operator)? Flag documents that seem to address a different audience without stating it.

### Step 3 — Produce the report

Write the report in the same language as the majority of analyzed documents (detect language from document content).

Follow this template strictly:

```
# Corpus Structural and Consistency Analysis

Analysis date: {today's date}
Scope: {folder or glob pattern analyzed}
Documents analyzed: {total count}
Total lines: {sum of all document line counts}
Consistency score: {Excellent / Good / Needs Work / Inconsistent}

---

## Document inventory

| # | File | Title | Type | Lines | Version | Sections | Referenced by |
|---|------|-------|------|-------|---------|----------|---------------|
| 1 | {relative path} | {top title} | {main/annex/overview} | {count} | {version} | {count} | {count} |
| ... | ... | ... | ... | ... | ... | ... | ... |

---

## Summary

{3-5 sentences: overall health of the corpus, main structural issues, key strengths, most urgent fixes.}

---

## A — Structure and naming

### A1 -- File naming convention
{Pattern(s) identified. Outliers flagged.}

### A2 -- Numbering gaps
{Gaps found in sequences. If none: "No gaps found."}

### A3 -- Section heading consistency
{Dominant pattern and deviations.}

### A4 -- Mandatory sections
{Expected sections and documents missing them.}

### A5 -- Section ordering
{Ordering inconsistencies across same-type documents.}

---

## B — Content quality

### B1 -- Duplicate content
{Verbatim or near-verbatim duplicates with file:line locations.}

### B2 -- Terminology consistency
{Terminology table and inconsistencies.}

### B3 -- Version and date consistency
{Version/date comparison across documents.}

### B4 -- Placeholder detection
{All placeholders found with file:line locations.}

---

## C — Cross-references

### C1 -- Broken references
{References pointing to non-existent files.}

### C2 -- Orphan documents
{Documents not referenced by any other.}

### C3 -- Circular reference chains
{Chains of 3+ documents.}

### C4 -- Reference depth violation
{References pointing to sections instead of documents.}

### C5 -- Reference format consistency
{Dominant format and deviations.}

---

## D — Coverage and completeness

### D1 -- Topic coverage matrix

| Topic | {Doc 1} | {Doc 2} | {Doc 3} | ... |
|-------|---------|---------|---------|-----|
| {topic} | {yes/no/partial} | ... | ... | ... |

{Gaps identified.}

### D2 -- Depth imbalance
{Outlier documents by size.}

### D3 -- Missing counterparts
{Missing symmetric documents.}

### D4 -- Root document / table of contents
{Index completeness assessment.}

---

## E — Architecture

### E1 -- Responsibility boundaries
{Overlapping ownership between documents.}

### E2 -- Layering
{Layer analysis: overview -> detail -> annex.}

### E3 -- Audience consistency
{Audience analysis across documents.}

---

## Cross-reference map

{Visual or tabular representation of which document references which:}

| Document | References |
|----------|-----------|
| {Doc A} | {Doc B, Doc C} |
| {Doc B} | {Doc A} |
| ... | ... |

---

## Synthesis table

| # | Category | Finding | Severity | File(s) |
|---|----------|---------|----------|---------|
| 1 | {A1..E3} | {Short description} | {High/Medium/Low} | {file(s)} |
| ... | ... | ... | ... | ... |

---

## Priority recommendations

{Numbered list of the 5-10 most important actions, ranked by decreasing impact. Each recommendation includes concrete guidance on how to fix the issue.}
```

## Consistency score

Derive the Consistency score from the synthesis table and severity distribution:

- Excellent: No High or Medium issues; at most a few Low.
- Good: No High issues; few Medium; some Low acceptable.
- Needs Work: One or more High issues, or many Medium.
- Inconsistent: Multiple High issues, or systematic problems across categories.

## Severity definitions

- High: Broken reference, missing counterpart document, duplicate rules creating ambiguity, orphan critical document, naming convention violation breaking tooling.
- Medium: Missing mandatory section, terminology inconsistency, placeholder content, depth imbalance, reference format inconsistency.
- Low: Section ordering variation, cosmetic naming deviation, minor audience mismatch.

## Report rules

- Always cite exact file names and line numbers when possible.
- For High and Medium issues, provide a concrete remediation action.
- Do not flag false positives. When uncertain, phrase the finding as a question.
- The topic coverage matrix (D1) should focus on the 10-15 most important topics, not try to be exhaustive.
- Adapt severity labels to the document language (e.g., French: Haute/Moyenne/Basse).
