# LOE Document

Reference for generating a Level of Effort document estimating work effort, resources, and costs.

---

## Purpose

Create a structured Level of Effort (LOE) document estimating work effort, resources, and costs for a given task or project. Covers scope, business impact, resource requirements, effort breakdown, risks, dependencies, and assumptions.

## Scope

Input: a description of a task, project, or initiative to estimate.

## Process

### Step 1 -- Analyze the task

Read the input fully. Map out all key components: requirements, dependencies, risks, and effort estimation factors. Consider business priorities and risk appetite based on the organization's context.

### Step 2 -- Produce the LOE document

Output in Markdown. No bold or italic formatting (no asterisks).

The document must contain these 9 sections in order:

### Section 1: Task Overview

- Provide a high-level summary of the task, project, or initiative being estimated.
- Define objectives and expected outcomes.
- Identify key stakeholders and beneficiaries.

### Section 2: Business Impact

- Define the business problem this task is addressing.
- List expected benefits and value to the organization.
- Highlight any business risks or regulatory considerations.

### Section 3: Scope and Deliverables

- Outline in-scope and out-of-scope work.
- Break down major deliverables and milestones.
- Specify acceptance criteria for successful completion.

### Section 4: Resource Requirements

- Identify required skill sets and roles (e.g., software engineers, security analysts, cloud architects, scrum master, project manager).
- Estimate the number of personnel needed, in tabular format.
- List tooling, infrastructure, or licenses required.

Personnel table format:

| Role | Count | Justification |

### Section 5: Estimated Effort

- Break down tasks into granular units (e.g., design, development, testing, deployment).
- Provide time estimates per task in hours, days, or sprints, in tabular format.
- Aggregate total effort for the entire task or project.
- Include buffer time for unforeseen issues or delays.
- Use T-shirt sizing (S/M/L/XL) or effort points to classify work complexity.

Effort table format:

| Task | Estimate | Complexity |

### Section 6: Dependencies

- List external dependencies (e.g., APIs, third-party vendors, internal teams).
- Specify hardware/software requirements that may impact effort.

### Section 7: Risks and Mitigations

- Identify technical, security, or operational risks that could affect effort.
- Propose mitigation strategies to address risks.
- Indicate if risks could lead to effort overruns.

### Section 8: Assumptions and Constraints

- List key assumptions that influence effort estimates.
- Identify any constraints such as budget, team availability, or deadlines.

### Section 9: Questions and Open Items

- List outstanding questions or clarifications required to refine the LOE.
- Highlight areas needing further input from stakeholders.

## Output Format

Markdown document with no bold or italic formatting (no asterisks). Nine numbered sections in order: Task Overview, Business Impact, Scope and Deliverables, Resource Requirements, Estimated Effort, Dependencies, Risks and Mitigations, Assumptions and Constraints, Questions and Open Items. Use tabular format for personnel and effort breakdowns.

## Constraints

- No bold or italic formatting (no asterisks) in the output.
- Personnel estimates must be in tabular format.
- Time estimates must be in tabular format.
- Include buffer time for unforeseen issues or delays.
- Use T-shirt sizing (S/M/L/XL) or effort points for complexity classification.
