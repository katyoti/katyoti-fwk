---
name: team-design-patterns
description: >-
  Team structure design methodology for project execution. Teaches agents
  how to map project plans to team compositions with roles, responsibilities,
  milestone assignments, and communication patterns.
iconName: users-group
icon: "👥"
hashtags: [planning, team, roles, design]
category: methodology
provides: [task]
---

# Team Design Patterns

A methodology for designing team structures that match project plans.

## Core Principles

1. **Plan drives team** -- The team structure must serve the plan, not the other way around. Start from milestones and tasks, then determine who is needed.
2. **One owner per deliverable** -- Every milestone and major task needs exactly one person responsible. Shared ownership means no ownership.
3. **Skill-based assignment** -- Match people to tasks based on required skills, not availability. Identify gaps early.
4. **Communication scales quadratically** -- Every person added increases communication overhead. Prefer smaller teams with clear interfaces.
5. **Parallel work needs clear boundaries** -- When multiple people work simultaneously, define who owns what files/modules/areas.

## Team Design Process

### Step 1: Extract roles from the plan

For each milestone and task, identify what skills are needed:
- What technical expertise is required? (frontend, backend, devops, design, etc.)
- What domain knowledge is needed?
- Are there specialized tasks that need dedicated attention? (security, performance, etc.)

### Step 2: Define roles

Each role should have:
- **Title** -- Clear, descriptive (e.g., "Backend Developer", not "Developer 2")
- **Primary responsibilities** -- What this person owns
- **Milestone focus** -- Which milestones this role is critical for
- **Skills required** -- Technical and domain skills needed

### Step 3: Map roles to milestones

Create a matrix showing which roles are active in each milestone:
- Early milestones: architecture and infrastructure roles dominate
- Middle milestones: feature development roles dominate
- Late milestones: QA, polish, and launch roles dominate

### Step 4: Define communication patterns

Based on team size and structure:
- **2-4 people**: Daily standups, weekly reviews. Direct communication.
- **5-8 people**: Daily standups, weekly sprint planning, bi-weekly reviews. Sub-teams may form.
- **9+ people**: Sub-teams required. Team leads coordinate. Weekly cross-team syncs.

Communication frequency should match the milestone phase:
- Foundation: more communication (alignment needed)
- Implementation: less communication (heads-down work)
- Integration/launch: more communication (coordination needed)

## Team Size Guidelines

| Team Size | Communication | Coordination | Best For |
|-----------|--------------|--------------|----------|
| 2-3 | Direct | Informal | Small MVPs, prototypes |
| 4-6 | Structured | Lightweight | Standard projects, startups |
| 7-10 | Sub-teams | Formal | Mid-size products, multi-feature |
| 10+ | Hierarchical | Heavy | Enterprise, multi-team |

## Quality Criteria

- Roles must match the plan's actual tasks and skill requirements
- Team size must fit the stated constraints (budget, headcount)
- Every milestone must have a clear lead and supporting roles
- Communication patterns must be appropriate for the team size
- Do NOT invent roles that the plan doesn't need
- When done, call **complete** with the FULL team structure in the summary — include ALL roles, responsibilities, milestone assignments, and communication patterns

## Anti-patterns

- **Everyone does everything** -- No clear ownership leads to dropped balls
- **Hero dependency** -- One person knows everything, becomes a bottleneck
- **Communication overload** -- Too many meetings, not enough building
- **Role inflation** -- Creating roles that don't have enough work to justify them
- **Ignoring constraints** -- Designing a 10-person team when the budget supports 4
