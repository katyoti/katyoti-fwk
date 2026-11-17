# Prose Analysis (Standard)

Reference for evaluating writing quality across three dimensions with A-F ratings.

---

## Purpose

Evaluate writing quality across novelty, clarity, and prose dimensions using A-F rating scales with specific criteria. Produces ratings, improvement recommendations, and an overall score.

## Scope

Input: any written content (essay, article, blog post, draft, etc.).
Output: Markdown report with section headers, ratings, recommendations, and overall score.

## Process

### Step 1 — Understand intent

Identify the writer's likely intent — what they wanted to convey to the reader.

### Step 2 — Rate NOVELTY

Identify each discrete idea and evaluate how surprising, fresh, or novel the ideas are. Content is novel if it combines ideas in an interesting way, proposes something new, or describes a vision/application that hasn't been discussed this way before.

Rate liberally — if the content meets criteria for a higher tier, give it that tier.

- "A - Novel" (~90%+) — Does one or more of: introduces new ideas; proposes a new framework well-structured and supported by argument; introduces new models for understanding the world; makes a clear prediction backed by strong concepts/data; introduces a new vision of the future; introduces a new way of thinking about reality; recommends behavior based on a new proposed way of thinking.
- "B - Fresh" (~80-90%) — Proposes new ideas but doesn't reach the A tier. Minor expansion on existing ideas in a useful way.
- "C - Incremental" (~50-80%) — Useful expansion of existing ideas or useful description of the past, but no new ideas. Valuable resource collections; observations and takeaways from past descriptions.
- "D - Derivative" (~20-50%) — Largely derivative of well-known ideas. Contains ideas or facts, but nothing new.
- "F - Stale" (<20%) — No new ideas. Random ramblings that say nothing new.

### Step 3 — Rate CLARITY

- "A - Crystal" — Very clear and concise argument, stays in flow, doesn't lose the main problem and solution.
- "B - Clean" — Quite clear and concise, only needs minor optimizations.
- "C - Kludgy" — Good ideas, but could be more concise and clearer about problems and solutions.
- "D - Confusing" — Quite confusing, not clear how the pieces connect.
- "F - Chaotic" — Not even clear what's being attempted.

### Step 4 — Rate PROSE

- "A - Inspired" — Clear, fresh, distinctive prose free of cliche.
- "B - Distinctive" — Strong writing, lacks significant cliche use.
- "C - Standard" — Decent prose, lacks distinctive style and/or uses too much cliche or standard phrases.
- "D - Stale" — Significant use of cliche and/or weak language.
- "F - Weak" — Overwhelming language weakness and/or cliche use.

### Step 5 — Recommendations

Create a bulleted list of recommendations on how to improve each rating, each no more than 16 words.

### Step 6 — Overall rating

The overall rating is the lowest of the three ratings (NOVELTY, CLARITY, PROSE). Output only the letter grade, no additional information.

## Output Format

Markdown with section headers followed by content. Sections in order: NOVELTY, CLARITY, PROSE, Recommendations, Overall rating.

## Constraints

- Output Markdown with section headers followed by content. No bold or italic formatting.
- The overall rating cannot be higher than the lowest individual rating.
