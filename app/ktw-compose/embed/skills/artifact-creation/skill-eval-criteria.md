---
name: skill-eval-criteria
description: "The criteria for judging a skill: triggering accuracy, performance, budget, time, UX, and output quality, plus the severity rubric. Load to define 'good' before evaluating a skill."
advertised: false
hashtags: [evaluation, criteria, skill, create-items]
category: guide
---

# What makes a skill good

Before evaluating a skill, fix what you are measuring. A skill is judged on six
dimensions; score each with the severity rubric below, always citing evidence.

## The seven dimensions

0. **Triggering accuracy** — the #1 skill-specific criterion: does the skill
   get LOADED when its situation arises, and stay silent when it doesn't?
   Test both directions: a run whose request matches the description must
   show the skill loaded BEFORE the work started (not improvised around);
   a near-miss request must NOT load it. A skill that never fires has zero
   value regardless of its body; one that fires on everything pollutes
   every run. The lever is almost always the `description` (what + when +
   anti-triggers).
1. **Performance** — turns and tool calls to reach the result. Fewer is better;
   repeated identical calls or re-asking for known data is waste.
2. **Budget** — input/output tokens and USD cost. Compare against the per-task /
   per-mode / per-tool thresholds in
   `ktw-compose:/reference/artifact/eval/budgets.md`.
3. **Time** — wall-clock and time-to-first-token. A skill that stalls or polls
   without backoff burns time the user feels.
4. **User UX** — did it ask the *right* questions, the right number of rounds,
   with clear phrasing? Did it confirm before acting? Over-interviewing and
   vague questions both count against it. (For a skill whose value is the
   interaction, this dimension dominates.)
5. **Output quality vs the skill's stated purpose** — does the result do what the
   skill's `description` promised? This is the anchor: a fast, cheap run that
   produces the wrong thing still fails.
6. **Size vs minimum (conciseness)** — MEASURE the skill body word count, then
   estimate the **minimum** that still conveys the full methodology + every
   contract (steps, gates, output template). A skill is loaded verbatim into
   context, so every word past the minimum is recurring cost. Report
   `words / est-minimum` and name the excess concretely (filler prose, repetition
   across sections, restating what a tool description already says, examples
   longer than they need to be). ✓ when close to minimum; ✗ wasteful when bloated
   (e.g. 2× the minimum); also flag UNDER-size (too thin to carry the method).
   The lever: cut to the smallest body that still teaches the method.

## Severity rubric

Tag every judgement with exactly one — the catalog of per-category examples is
`ktw-compose:/reference/artifact/eval/run-rubric.md`:

| Tag | Meaning |
|---|---|
| ✓ good | worked as intended, reasonable cost, clear outcome |
| ⚠ acceptable | works with minor friction (one retry, slight over-fetch) |
| ✗ wasteful | significant inefficiency (3+ identical calls, re-asking known data, polling without backoff) |
| ✗✗ failed | did not achieve its goal (errors, abandoned work, ended a turn with no tool call) |

## Evidence rule

Every verdict MUST cite concrete data — turn counts, token numbers, the specific
tool call or question. "The skill did well" is not an evaluation; "✓ good — drove
to a confirmed output in 4 turns (1107 in / 942 out tokens), 4 ask-user rounds
each with a distinct question" is.

## The anchor

When dimensions conflict, **output quality vs purpose wins**, then UX (for
interactive skills), then performance/budget/time. A skill is not "efficient" if
it is efficiently producing the wrong result.
