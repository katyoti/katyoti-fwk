# Technical Document Analysis

Reference for analyzing a single technical Markdown document.

---

## Purpose

Analyze a technical Markdown document for errors, inconsistencies, standard compliance, exhaustiveness, and suggest improvements. Reads sibling documents in the same folder for cross-reference checks.

## Scope

Input: a single technical Markdown document.
Allowed tools: Read, Grep, Glob.
argument-hint: [file-path]

The analysis reads the target file and all sibling `.md` files in the same folder for cross-reference consistency checks.

## Process

### Step 1 — Collect context

1. Read the target document in full.
2. Glob `*.md` in the same folder to list sibling documents.
3. Read all sibling documents to build cross-reference context (structure, terminology, rules, values, references).

### Step 2 — Run all analysis categories

Execute every category below. Never skip one. If no issue is found for a check, state it explicitly.

#### Category A — Internal consistency

- A1 -- Rules vs examples: Do the examples provided in the document comply with its own stated rules?
- A2 -- Cross-section contradictions: Do two sections of the document contradict each other?
- A3 -- Incomplete content / placeholders: Detect `XX`, `TODO`, `TBD`, `TBC`, `N/A` used as placeholder, incomplete dates, empty sections, `...` ellipsis standing for missing content.
- A4 -- Ambiguities: Vague terms without a clear default or decision ("optional", "as needed", "depending on configuration", "may", "can be").

#### Category B — Standard compliance

- B1 -- Applicable standards: Identify technical standards relevant to the document's subject (RFC, ISO, W3C, OASIS, etc.) and assess compliance.
- B2 -- Documented deviations: Are intentional deviations from standards explicitly stated and justified?
- B3 -- Data formats: Do dates, numbers, encodings, identifiers follow a recognized standard? Are the chosen formats fully specified (precision, timezone, edge cases)?

#### Category C — Cross-document consistency

- C1 -- Terminology: Does the document use the same terms as sibling documents for the same concepts? Flag synonyms or inconsistent naming.
- C2 -- Values and rules: Are values, formats, and rules defined here consistent with those in sibling documents covering related topics?
- C3 -- Cross-references: Are sibling documents that should be referenced actually referenced? Do existing references point to real files? Are references limited to document-level (no deep section links)?

#### Category D — Exhaustiveness

- D1 -- Missing sections: Compared to sibling documents of the same type, are expected sections absent (e.g., References, Overview, Validation, Examples, Version footer)?
- D2 -- Edge cases not covered: Do the rules and examples cover the important edge cases of the domain?
- D3 -- Related information absent: Are directly related topics omitted (compression, size limits, forbidden characters, error handling, etc.)?
- D4 -- Metadata alignment: If the document describes a data format, is coherence with associated metadata or manifest ensured?

#### Category E — Quality and improvements

- E1 -- Example quality: Are examples varied enough to illustrate all rules, including edge cases? Suggest concrete enriched examples.
- E2 -- Summary tables: Would a summary table be useful to condense rules, type mappings, or decision matrices? Propose one if so.
- E3 -- Rule clarity: Are rules unambiguous and actionable for a developer implementing them?
- E4 -- Structure and readability: Is the heading hierarchy logical? Are sections in a coherent order?

### Step 3 — Produce the report

Write the report in the same language as the analyzed document (detect language from the document content).

Follow this template strictly:

```
# Document Analysis: {file name}

Analysis date: {today's date}
Analyzed document: {relative file path}
Sibling documents consulted: {list of sibling files read}

---

## Summary

{2-3 sentences summarizing the main conclusions: issue count by severity, document strengths, major improvement areas.}

---

## A — Internal consistency

### A1 -- Rules vs examples
{Detailed findings with line numbers. If none: "No issues found."}

### A2 -- Cross-section contradictions
{Detailed findings with line numbers. If none: "No issues found."}

### A3 -- Incomplete content / placeholders
{Detailed findings with line numbers. If none: "No issues found."}

### A4 -- Ambiguities
{Detailed findings with line numbers. If none: "No issues found."}

---

## B — Standard compliance

### B1 -- Applicable standards
{List of applicable standards and compliance assessment.}

### B2 -- Documented deviations
{Deviations found and whether they are justified.}

### B3 -- Data formats
{Analysis of formats used.}

---

## C — Cross-document consistency

### C1 -- Terminology
{Terminology discrepancies with sibling documents.}

### C2 -- Values and rules
{Value/rule contradictions with sibling documents.}

### C3 -- Cross-references
{Missing, broken, or unnecessary references.}

---

## D — Exhaustiveness

### D1 -- Missing sections
{Expected sections absent, compared to sibling documents.}

### D2 -- Edge cases not covered
{Domain edge cases not addressed.}

### D3 -- Related information absent
{Related topics not mentioned.}

### D4 -- Metadata alignment
{Coherence with metadata or manifest, if applicable.}

---

## E — Quality and improvements

### E1 -- Example quality
{Example analysis and enrichment suggestions.}

### E2 -- Summary tables
{Useful tables to add, with concrete proposal if relevant.}

### E3 -- Rule clarity
{Ambiguous or non-actionable rules.}

### E4 -- Structure and readability
{Heading hierarchy and organization analysis.}

---

## Synthesis table

| # | Category | Finding | Severity | Line(s) |
|---|----------|---------|----------|----------|
| 1 | {A1..E4} | {Short description} | {High/Medium/Low} | {line(s)} |
| ... | ... | ... | ... | ... |

---

## Priority recommendations

{Numbered list of the 3-5 most important actions, ranked by decreasing impact. Each recommendation includes a concrete correction or addition suggestion.}
```

## Severity definitions

- High: Factual inconsistency, contradiction, false or misleading information, broken reference.
- Medium: Missing information, ambiguity, standard non-compliance, missing section.
- Low: Formatting improvement, example enrichment, added reference, cosmetic.

## Report rules

- Always include line numbers when possible.
- For every High or Medium issue, propose a concrete fix or addition.
- Do not flag false positives. When uncertain, phrase the finding as a question.
- Adapt severity labels to the document language (e.g., French: Haute/Moyenne/Basse).
