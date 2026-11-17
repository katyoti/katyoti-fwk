---
name: interview-technique
description: >-
  Structured interview methodology for goal clarification. Teaches agents
  how to conduct open-ended interviews, probe for specifics, summarize
  findings, and confirm understanding with the user.
iconName: messages
icon: "💬"
hashtags: [interview, goals, clarification]
category: methodology
provides: [task]
---

# Interview Technique

A structured methodology for interviewing users to clarify goals, requirements, and constraints.

## Core Principles

1. **Open before closed** -- Start with broad, open questions. Narrow down with specific probes once you have context.
2. **One question at a time** -- Never ask multiple questions in a single prompt. Each ask-user call should have exactly one question.
3. **Summarize before confirming** -- Always present a structured summary of your understanding before asking for confirmation.
4. **Listen for what's missing** -- Pay attention to what the user does NOT say. Probe gaps.
5. **Respect the user's time** -- Aim for 3-5 rounds maximum. Don't over-interview.

## Interview Structure

### Round 1: Open exploration
Ask the user to describe their goal in their own words. Do not constrain the format.

Example: "Please describe your goal in your own words. What are you trying to accomplish?"

### Round 2: Clarifying probes
Based on the first response, ask targeted questions about:
- **Scope**: What's included and what's explicitly excluded?
- **Audience**: Who is this for?
- **Constraints**: Budget, timeline, team, technology?

Combine these into a focused follow-up. Ask about the areas the user left vague.

### Round 3: Success criteria
Ask how the user will know the goal is achieved. Push for measurable, specific criteria.

Example: "How will you measure success? What specific outcomes would tell you this worked?"

### Round 4: Summary and confirmation
Present a structured summary with:
- Goal statement (1-2 sentences)
- Success criteria (3-5 measurable items)
- Key constraints (budget, timeline, team, technology)

Get explicit approval with request-validation (kind "approve") when it is offered, else ask-user with type "confirm".

### Round 5 (if needed): Adjustments
If the user rejects the summary, ask what needs adjusting. Update and re-confirm.

## Output Format

The interview should produce a structured result containing:

```
Goal: [clear, specific goal statement]
Success Criteria:
1. [measurable criterion]
2. [measurable criterion]
3. [measurable criterion]
Constraints:
- [key constraint with value]
- [key constraint with value]
```

## Quality Criteria

- The goal statement must be specific and actionable, not vague
- Success criteria must be measurable (numbers, dates, percentages)
- Constraints must include all limits the user mentioned (budget, timeline, team, technology)
- The user must explicitly confirm the summary before completion
- Aim for 3-5 interview rounds maximum — respect the user's time
- When done, call **complete** with the FULL structured result in the summary — include ALL data (goal, criteria, constraints). Do NOT use generic summaries.

## Anti-patterns

- **Interrogation mode**: Asking too many questions in rapid succession without acknowledging responses
- **Leading questions**: Suggesting answers instead of letting the user express their needs
- **Premature closure**: Moving to confirmation before enough detail is gathered
- **Question overload**: Presenting 5+ sub-questions in a single prompt
