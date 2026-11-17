# Summary

Reference for generating a structured summary from any content.

---

## Purpose

Summarize any content into a structured format with a one-sentence summary, main points, and takeaways.

## Scope

Input: any content (article, document, transcript, notes, etc.).

## Process

### Step 1 -- Analyze the content

Read the input fully. Identify the core message, key points, and most valuable takeaways.

### Step 2 -- Output the summary

Output in Markdown using numbered lists (not bullets).

The document must contain these three sections in order:

### ONE SENTENCE SUMMARY:

A single sentence of 20 words combining the full understanding of the content.

### MAIN POINTS:

The 10 most important points of the content. Each point: no more than 16 words.

### TAKEAWAYS:

The 5 best takeaways from the content.

## Output Format

Markdown document with numbered lists (not bullets). Three sections in order: ONE SENTENCE SUMMARY (single sentence, 20 words), MAIN POINTS (10 items, max 16 words each), TAKEAWAYS (5 items).

## Constraints

- ONE SENTENCE SUMMARY must be exactly 20 words.
- MAIN POINTS must contain exactly 10 items, each no more than 16 words.
- TAKEAWAYS must contain exactly 5 items.
- Use numbered lists, not bullets.
- Do not repeat items across output sections.
- Do not start items with the same opening words.
