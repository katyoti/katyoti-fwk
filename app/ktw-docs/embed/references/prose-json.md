# Prose Analysis (JSON)

Reference for evaluating writing quality with A-F ratings, outputting results as a structured JSON object.

---

## Purpose

Evaluate writing quality across novelty, clarity, and prose dimensions using A-F rating scales. Output a machine-readable JSON object.

## Scope

Input: any written content (essay, article, blog post, draft, etc.).
Output: a single JSON object (no code fences, no markdown, no additional text).

## Process

### Step 1 — Understand intent

Identify the writer's likely intent — what they wanted to convey to the reader.

### Step 2 — Rate NOVELTY

Identify each discrete idea and evaluate novelty. Rate liberally — if criteria for a higher tier are met, give that tier.

- "A - Novel" (~90%+) — Introduces new ideas; proposes a new framework/model; makes predictions backed by data; proposes new explanations for known phenomena; lays out a well-supported vision of the future; recommends behavior based on new thinking.
- "B - Fresh" (~80-90%) — Proposes new ideas but doesn't reach A tier. Minor useful expansion on existing ideas.
- "C - Incremental" (~50-80%) — Useful expansion of existing ideas or insightful description of the past. Useful resource collections; observations with takeaways; minor expansions.
- "D - Derivative" (~20-50%) — Largely derivative. Restatement of common knowledge; rehashes without new takes.
- "F - Stale" (<20%) — No new ideas. Completely trite, heavily cliche.

### Step 3 — Rate CLARITY

- "A - Crystal" — Very clear, concise, stays in flow.
- "B - Clean" — Quite clear, only minor optimizations needed.
- "C - Kludgy" — Good ideas, could be more concise and clear.
- "D - Confusing" — Unclear how pieces connect.
- "F - Chaotic" — Not clear what's being attempted.

### Step 4 — Rate PROSE

- "A - Inspired" — Clear, fresh, distinctive, cliche-free.
- "B - Distinctive" — Strong writing, lacks significant cliche.
- "C - Standard" — Decent but lacks distinctive style, too much cliche.
- "D - Stale" — Significant cliche and/or weak language.
- "F - Weak" — Overwhelming weakness and/or cliche.

### Step 5 — Produce JSON output

Output ONLY a JSON object (no code fences, no other text). The overall-rating is the lowest of the three ratings.

```json
{
  "novelty-rating": "A - Novel",
  "novelty-rating-explanation": "Combines multiple existing ideas and adds new ones to construct a vision of the future.",
  "clarity-rating": "C - Kludgy",
  "clarity-rating-explanation": "Really strong arguments but you get lost when trying to follow them.",
  "prose-rating": "A - Inspired",
  "prose-rating-explanation": "Uses distinctive language and style to convey the message.",
  "recommendations": "Bulleted list of improvements, each no more than 16 words.",
  "one-sentence-summary": "A 20-word one-sentence summary of the overall quality based on the ratings and explanations.",
  "overall-rating": "C"
}
```

## Constraints

- Output ONLY the JSON object — no code fences, no markdown, no additional text.
- Each explanation field: 15-20 words.
- one-sentence-summary: 20 words.
- recommendations: bulleted list, each item no more than 16 words.
- overall-rating: letter grade only (lowest of the three ratings), no tagline.
