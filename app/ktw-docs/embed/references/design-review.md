# Design Review

Reference for reviewing architecture and system design documents.

---

## Purpose

Conduct a detailed architecture and design review with actionable recommendations. Evaluates across eight dimensions covering architecture, integrations, security, performance, data, maintainability, risks, and readability.

## Scope

Input: an architecture document, design specification, system diagram description, or technical design to review.
Output: Markdown report with one section per dimension, findings, recommendations, and a summary.

## Process

### Step 1 — Analyze the design

Read the full input. Identify the system's purpose, its components, external integrations, data flows, and any diagrams or visual descriptions provided.

### Step 2 — Evaluate across eight dimensions

Assess the design against each of the following areas. For each, identify strengths, weaknesses, and concrete improvements.

#### 1. Architecture Clarity and Component Design

- Analyze all diagrams, internal components, and external systems.
- Assess whether roles and responsibilities of each component are well-defined.
- Evaluate whether interactions between components are efficient, logical, and well-documented.
- Identify redundancy, unnecessary complexity, or unclear responsibilities.

#### 2. External System Integrations

- Evaluate all integrations to external systems.
- Assess security, performance, and reliability of each integration.
- Determine whether the system handles diverse external clients without compromising performance or security.

#### 3. Security Architecture

- Assess authentication, authorization, and data protection mechanisms.
- Identify weaknesses in access control and potential attack vectors.
- Suggest hardening improvements following security best practices.

#### 4. Performance, Scalability, and Resilience

- Analyze how the design ensures high performance (rate limiting, caching, connection pooling, etc.).
- Evaluate horizontal scalability — can the system handle increasing clients/load? Where are the bottlenecks?
- Assess fault tolerance and resilience. What happens when a specific component fails?

#### 5. Data Management and Storage Security

- Review how data is handled, stored, and encrypted.
- Assess whether data flow between components is optimized and secure.
- Evaluate data segregation and client isolation — risk of data leaks or breaches.

#### 6. Maintainability, Flexibility, and Future Growth

- Evaluate modularity and ease of maintenance (containerization, microservices, etc.).
- Assess how easily new clients can be onboarded or new features added without significant rework.
- Suggest strategies to future-proof the architecture.

#### 7. Risks and Limitations

- Highlight dependencies on third-party services, single points of failure, security vulnerabilities, and performance bottlenecks.
- Provide actionable recommendations for each identified risk.

#### 8. Document Readability

- Flag inconsistencies in the document and its vocabulary.
- Identify sections that are unclear, ambiguous, or need rewriting.

### Step 3 — Output

Produce the review in Markdown with the following structure:

1. One section per evaluation dimension (numbered 1-8 as above), each containing:
   - Findings (strengths and weaknesses)
   - Specific, actionable recommendations

2. A concluding section titled Summary that:
   - Lists the design's key strengths
   - Lists the most critical areas where adjustments would have the highest positive impact

## Constraints

- Use plain Markdown — no bold or italics in the output.
- Every weakness must be paired with an actionable recommendation.
- Do not invent issues that are not present in the design.
