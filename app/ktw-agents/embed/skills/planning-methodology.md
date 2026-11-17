---
name: planning-methodology
description: >-
  Structured planning methodology for breaking goals into milestones,
  tasks, and subtasks. Teaches agents how to create realistic,
  constraint-aware project plans with proper scoping.
iconName: list-check
icon: "📋"
hashtags: [planning, methodology, milestones]
category: methodology
---

# Planning Methodology

A structured approach to building project plans from clarified goals.

## Core Principles

1. **Constraint-first** -- The plan must fit within stated constraints (budget, timeline, team). Start by understanding limits before designing scope.
2. **Milestone = deliverable** -- Each milestone must produce something usable or testable. Never define milestones as time periods alone.
3. **Subtask independence** -- Subtasks within a milestone should be assignable to different team members without blocking each other.
4. **Risk front-loading** -- Put the hardest, most uncertain work in early milestones. Later milestones should be polish and optimization.
5. **User validation points** -- Every plan needs moments where the user can verify progress before continuing.

## Planning Structure

### Step 1: Scope from constraints

Given the constraints (budget, timeline, team size), determine what's achievable:
- Timeline drives milestone count (1 milestone per month is typical)
- Team size drives parallelism (more people = more parallel subtasks)
- Budget drives scope (cut features, not quality)

### Step 2: Define milestones (2-4)

Each milestone needs:
- **Name** that describes the deliverable, not the phase
- **Outcome** statement: what exists when this milestone is done
- **Duration** in weeks
- **Expected outcome** that can be demonstrated or verified

Typical milestone progression:
1. Foundation (architecture, setup, core infrastructure)
2. Core features (the main value proposition)
3. Polish and integration (quality, performance, secondary features)
4. Launch preparation (testing, deployment, documentation)

### Step 3: Break milestones into tasks and subtasks

Each task should be:
- **Specific** -- "Implement user authentication with OAuth" not "Set up auth"
- **Estimable** -- Small enough to estimate in days, not weeks
- **Assignable** -- One person can own it

Subtasks add detail without creating new tracking items:
- Subtask = steps within a task
- 2-4 subtasks per task is ideal

### Step 4: Validate against constraints

Check the plan against reality:
- Does the total effort fit the timeline with the available team?
- Are there dependencies that create bottlenecks?
- Is there buffer for unexpected issues (aim for 80% utilization)?
- Can the user verify progress at each milestone?

## Anti-patterns

- **Kitchen sink planning** -- Trying to fit everything into the timeline. Cut scope, not quality.
- **Sequential everything** -- Plans where every task blocks the next. Look for parallelism.
- **Vague milestones** -- "Phase 2: Development" tells you nothing. Name the deliverable.
- **No buffer** -- Plans with 100% utilization always slip. Leave 20% buffer.
- **Big bang delivery** -- Everything ships at the end. Deliver incrementally.
