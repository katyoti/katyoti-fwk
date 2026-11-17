# Requirements Review

Reference for analyzing requirements documents (SRS, BRS, PRD, user stories, functional specs).

---

## Purpose

Review a requirements document for completeness, testability, ambiguity, conflicts, and traceability. Produces a structured report with per-requirement findings and priority remediation actions.

## Scope

Input: any requirements document — Software Requirements Specification (SRS), Business Requirements Specification (BRS), Product Requirements Document (PRD), user story backlog, functional specification, or similar.
Allowed tools: Read, Grep, Glob.
Output: Markdown report in the same language as the input document.

## Process

### Step 1 — Classify and inventory

1. Read the full document.
2. Identify the document type (SRS / BRS / PRD / user stories / mixed).
3. Extract every discrete requirement. Assign a temporary ID (R-01, R-02, …) to any requirement that lacks one.
4. Note which requirements have explicit IDs/labels and which do not.

### Step 2 — Run all analysis categories

Execute every category below. Never skip one. If no issue is found for a check, state it explicitly.

#### Category A — Completeness

- A1 -- Missing sections: For the detected document type, identify expected sections that are absent (e.g., for SRS: scope, definitions, functional requirements, non-functional requirements, constraints, assumptions; for PRD: goals, personas, user stories, success metrics, out-of-scope).
- A2 -- Requirement coverage gaps: Are there obvious user-facing features or system behaviors implied by the document scope that are not specified? List what appears to be missing.
- A3 -- Non-functional requirements: Are performance, security, scalability, availability, and accessibility requirements present? Flag missing NFR categories.
- A4 -- Assumptions and constraints: Are assumptions and constraints explicitly stated? Implicit assumptions (things that "everyone knows") are a risk — flag them.
- A5 -- Error and exception handling: Do functional requirements address failure modes, edge cases, and error responses, or do they only describe the happy path?

#### Category B — Testability

- B1 -- Vague or unmeasurable requirements: Flag requirements using unquantified terms: "fast", "user-friendly", "reliable", "high quality", "appropriate", "as needed", "reasonable", "adequate". Every such term is untestable.
- B2 -- Missing acceptance criteria: For each user story or functional requirement, is there a clear, verifiable acceptance criterion? Flag requirements with no testable pass/fail condition.
- B3 -- Compound requirements: Requirements containing "and" that bundle multiple behaviors — each sub-behavior needs its own testable condition. Flag and suggest splitting.
- B4 -- Measurable NFRs: Non-functional requirements must have numeric thresholds (e.g., "response time < 200ms under 1000 concurrent users"). Flag any NFR that lacks a measurable target.

#### Category C — Ambiguity and clarity

- C1 -- Ambiguous pronouns and references: "it", "this", "the system", "the user" used where the referent is unclear.
- C2 -- Undefined terms: Domain-specific terms, acronyms, or concepts used without definition. Flag any term that a new team member would need to guess at.
- C3 -- Weak modal verbs: "should", "may", "could", "might" used where "shall" or "must" is intended. Flag each occurrence — requirements with optional language are often skipped in implementation.
- C4 -- Ambiguous scope: Requirements whose scope is unclear — does this apply to all users? All environments? All data types? Flag when context is implicit.
- C5 -- Placeholders and TBDs: `TBD`, `TBC`, `TODO`, `XX`, `to be defined`, `to be determined` anywhere in the document.

#### Category D — Conflicts and consistency

- D1 -- Direct contradictions: Two requirements that cannot both be satisfied simultaneously. Cite both requirement IDs.
- D2 -- Implicit conflicts: Requirements that are individually valid but create tension under specific conditions (e.g., security requirement vs. usability requirement, performance target vs. data retention policy).
- D3 -- Inconsistent terminology: Same concept referred to by different names (e.g., "user" vs "customer" vs "client", "order" vs "purchase" vs "transaction"). Build a terminology conflict table.
- D4 -- Inconsistent priority or must/should usage: Requirements assigned conflicting priorities, or mixed use of priority language (MoSCoW, numbered priority, "critical/high/medium/low") without a single scheme.

#### Category E — Traceability

- E1 -- Missing requirement IDs: Requirements without a unique identifier cannot be traced in test plans, change logs, or issue trackers. Flag requirements that lack IDs.
- E2 -- Business goal linkage: Can each requirement be traced to a stated business goal, user need, or objective? Flag requirements that appear to have no stated justification.
- E3 -- Dependency declaration: Where a requirement depends on another being implemented first, is that dependency stated? Flag hidden sequencing dependencies.
- E4 -- Out-of-scope clarity: Is there an explicit out-of-scope section? Requirements documents without clear exclusions tend to grow scope silently.

### Step 3 — Produce the report

Write the report in the same language as the analyzed document.

Follow this template strictly:

```
# Requirements Review: {document title or file name}

Review date: {today's date}
Document type: {SRS / BRS / PRD / user stories / functional spec / mixed}
Total requirements identified: {count}
Requirements with explicit IDs: {count}

---

## Summary

{3-5 sentences: overall quality, dominant issues by category, strengths, most urgent fixes.}

---

## A — Completeness

### A1 -- Missing sections
{Findings. If none: "All expected sections present."}

### A2 -- Requirement coverage gaps
{Implied behaviors not specified.}

### A3 -- Non-functional requirements
{NFR categories present / missing.}

### A4 -- Assumptions and constraints
{Implicit assumptions and missing constraints.}

### A5 -- Error and exception handling
{Happy-path-only requirements flagged.}

---

## B — Testability

### B1 -- Vague or unmeasurable requirements
{List with requirement ID/text and the vague term flagged.}

### B2 -- Missing acceptance criteria
{Requirements with no verifiable pass/fail condition.}

### B3 -- Compound requirements
{Requirements to split, with suggested split.}

### B4 -- Measurable NFRs
{NFRs lacking numeric thresholds.}

---

## C — Ambiguity and clarity

### C1 -- Ambiguous pronouns and references
{Instances with line numbers.}

### C2 -- Undefined terms
{Terms needing definition.}

### C3 -- Weak modal verbs
{All "should/may/could/might" occurrences where "shall/must" was likely intended.}

### C4 -- Ambiguous scope
{Requirements with implicit or unclear scope.}

### C5 -- Placeholders and TBDs
{All instances with locations.}

---

## D — Conflicts and consistency

### D1 -- Direct contradictions
{Conflicting requirement pairs with IDs and explanation.}

### D2 -- Implicit conflicts
{Tension points between valid but incompatible requirements.}

### D3 -- Inconsistent terminology

| Concept | Terms Used | Recommended Term |
|---------|-----------|-----------------|
| {concept} | {term A, term B} | {pick one} |

### D4 -- Inconsistent priority scheme
{Conflicts or mixed schemes.}

---

## E — Traceability

### E1 -- Missing requirement IDs
{Count and list of unnumbered requirements.}

### E2 -- Business goal linkage
{Requirements with no stated justification.}

### E3 -- Dependency declaration
{Hidden sequencing dependencies found.}

### E4 -- Out-of-scope clarity
{Assessment of exclusion section.}

---

## Synthesis table

| # | Req ID | Category | Finding | Severity |
|---|--------|----------|---------|----------|
| 1 | {R-xx or "—"} | {A1..E4} | {Short description} | {High/Medium/Low} |

---

## Priority recommendations

{Numbered list of the 5-10 most important actions, ranked by decreasing impact. Each recommendation includes a concrete correction or rewrite suggestion.}
```

## Severity definitions

- High: Direct contradiction between requirements, untestable acceptance criteria, undefined critical term, missing core NFR category, TBD in a shipped-to-stakeholder document.
- Medium: Compound requirement needing split, weak modal verb, missing requirement ID, missing error handling, implicit assumption.
- Low: Structural suggestion, minor terminology inconsistency, cosmetic improvement.

## Report rules

- Always cite the requirement text or line number when flagging an issue.
- For every High or Medium issue, propose a concrete rewrite or addition.
- Do not flag stylistic preferences as issues — focus on structural and semantic problems.
- Adapt severity labels to the document language (e.g., French: Haute/Moyenne/Basse).
