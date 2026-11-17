# Design Document

Reference for generating a detailed software architecture design document using the C4 model.

---

## Purpose

Create a detailed design document with C4 architecture diagrams from a system description. Covers business posture, security posture, C4 diagrams (context, container, deployment) with Mermaid, risk assessment, and open questions.

## Scope

Input: a description of a system, application, or project idea.

## Process

### Step 1 -- Understand the system

Read the input fully. Map out the key concepts, components, users, data flows, and security requirements. Consider the organization's context (startup vs. enterprise risk appetite).

### Step 2 -- Produce the document

Output in Markdown, no bold or italic formatting (no asterisks).

The document must contain these sections in order:

### BUSINESS POSTURE

- Determine the business priorities and goals the system is trying to address.
- List the most important business risks that need to be addressed based on those priorities and goals.

### SECURITY POSTURE

- List all existing security controls (prefix each with "security control:").
- List all accepted risks (prefix each with "accepted risk:").
- Provide a list of recommended high-priority security controls not mentioned in the input.
- Provide a list of security requirements important for the system.

Focus on secure software development lifecycle and deployment model.

### DESIGN

#### C4 CONTEXT

Provide a Mermaid diagram showing the system as a box in the centre, surrounded by its users and interacting systems.

Below the diagram, provide a table with these columns:

| Name | Type | Description | Responsibilities | Security controls |

#### C4 CONTAINER

Provide a Mermaid diagram showing the high-level software architecture shape, how responsibilities are distributed, major technology choices, and how containers communicate.

Below the diagram, provide a table with the same columns:

| Name | Type | Description | Responsibilities | Security controls |

#### C4 DEPLOYMENT

Provide a Mermaid diagram showing how software instances and containers are deployed onto infrastructure within a given deployment environment.

Below the diagram, provide a table with the same columns:

| Name | Type | Description | Responsibilities | Security controls |

### RISK ASSESSMENT

Answer:

- What are the critical business processes to protect?
- What data is being protected and what is its sensitivity?

### QUESTIONS & ASSUMPTIONS

List open questions and default assumptions regarding business posture, security posture, and design decisions.

## Output Format

Markdown document with no bold or italic formatting (no asterisks). Sections in order: BUSINESS POSTURE, SECURITY POSTURE, DESIGN (C4 CONTEXT, C4 CONTAINER, C4 DEPLOYMENT each with Mermaid diagram and table), RISK ASSESSMENT, QUESTIONS & ASSUMPTIONS.

Each C4 table uses the columns: Name | Type | Description | Responsibilities | Security controls.

## Constraints

- No bold or italic formatting (no asterisks) in the output.
- Security controls must be prefixed with "security control:".
- Accepted risks must be prefixed with "accepted risk:".
- Each C4 level must include both a Mermaid diagram and a description table.
