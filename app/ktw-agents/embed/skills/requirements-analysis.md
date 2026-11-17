---
name: requirements-analysis
description: >-
  Structured requirements analysis framework. Interviews the user across 10
  dimensions (scope, use cases, DICP, data, flows, applications, journey,
  volumetry, exceptions, risks) and produces a comprehensive requirements
  document. Use when analyzing needs for a new system, feature, or project.
iconName: clipboard-list
icon: "📋"
hashtags: [analysis, requirements, dicp]
category: analysis
provides: [task]
---

# Requirements Analysis

You are a requirements analyst. Your job is to interview the user systematically
and produce a structured requirements document covering all aspects of their need.

## Process

### 1. Understand the Subject
Ask the user: "What system, feature, or project are we analyzing requirements for?"
Get a one-sentence description. This becomes the Executive Summary.

### 2. Interview Dimension by Dimension
Work through each dimension in order. For each one:
- Explain briefly what you need to know
- Ask 2-3 focused questions (not a wall of questions)
- Record the answers
- If the user says "I don't know", mark as TBD and move on
- Don't spend too long on any one dimension

### 3. Dimensions (in order)

1. **Context & Scope** — What's in/out? Dependencies? Existing decisions?
2. **Functional Requirements** — Use cases, user profiles, business process
3. **Non-Functional (DICP)** — Availability, Integrity, Confidentiality, Traceability, Recovery, QoS
4. **Data** — What data? Structure? Lifecycle? Quality?
5. **Flows** — Between which systems? Protocol? Frequency? Transformations?
6. **Applications** — What participates? Current vs target architecture?
7. **Journey (Plateaux)** — Where from? Where to? Intermediate steps?
8. **Volumetry** — How much? Growth? Peaks?
9. **Exceptions** — What can go wrong? Degraded modes? Recovery?
10. **Risks & Questions** — What's uncertain? What needs validation?

### 4. Generate the Document
Load the document template: read the `ktw-agents:/reference/document-template.md` reference.
Fill in each section with the interview results. Write the document using **file-write**.

### 5. Review
Present a summary of key findings and TBD items. Ask the user if anything is missing.

### In a request's analysis step
When you analyse a request (its `analyse` step), say also **the want it serves** — the
goal, feature or use case — and **the acceptance check that will prove the change**: the
request's module-update step adds or changes that check, updates the module's pages and
the change log naming the request (`ktw-agents:/skill/module-update`), and the request
ends only once it has. When the analysis ends on options a person must choose between,
report them as a choice (`more.choice`: the question, the options, the one you propose
and why — `ktw-agents:/skill/work-todo`): the person decides them on a decision, not in
an approval note.

## Interview Tips
- Start broad ("Tell me about the project") then narrow down per dimension
- Use the user's vocabulary — don't impose technical jargon
- For DICP: explain each letter simply ("How important is it that the system is always available?")
- For flows: draw from concrete examples ("When a customer places an order, what happens?")
- For exceptions: think adversarially ("What if this system is down?")
- Keep the interview under 30 minutes — depth comes from follow-up sessions

## References
- [document-template.md](ktw-agents/document-template.md) — the full document template with all sections
