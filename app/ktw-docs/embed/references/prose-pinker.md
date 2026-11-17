# Prose Analysis (Pinker)

Reference for assessing prose quality using Steven Pinker's Sense of Style principles.

---

## Purpose

Analyze prose quality using principles from Steven Pinker's The Sense of Style. Evaluates writing style, positive qualities, bad-writing pitfalls, provides examples, spelling/grammar fixes, and a scored assessment.

## Scope

Input: any written text (French or English).
Output: Markdown report with section headers. No bold or italic formatting.

## Language adaptation

- French text: Apply only language-agnostic principles — clarity, logical organization, avoiding unnecessary complexity, considering the reader's perspective, directness, coherence, and universal bad-writing pitfalls (nominalization, passive voice, redundancy, tautology, jargon, obfuscation, etc.). Adapt English-specific categories (e.g., adverbs ending in "-ly") to their French equivalents where applicable. Skip categories that don't translate (e.g., mangled English idioms).
- English text: Apply the full Pinker framework as described below.

## Process

### Step 1 — Analyze the writing

Read the full text. Identify the writer's intent and the style being used.

### Step 2 — Produce the output

Output in Markdown with section headers. No bold or italic formatting.

### STYLE ANALYSIS

Evaluate what style the prose is written in and what style it should be written in, based on Pinker's categories:

- Classic Style: clarity, directness, conversational tone, active voice, concrete nouns/verbs, simplicity
- Practical Style: efficient, clear, straightforward, utility over aesthetics
- Self-Conscious Style: foregrounding the writer's own thoughts, introspective, may detract from clarity
- Postmodern Style: skeptical, ironic, indirect, plays with conventions
- Academic Style: dense, formal, technical terminology, precision over readability
- Legal Style: meticulous detail, jargon-heavy, no ambiguity, complex sentences

Give 3-5 bullets of 16 words each.

### POSITIVE ASSESSMENT

Rate the prose 1-10 on each dimension. Give answers as 16-word bullets. The Importance weight guides the overall score:

- Clarity (Importance: 10) — intended message is clear to the reader
- Brevity (Importance: 8) — concise, avoids unnecessary words
- Elegance (Importance: 7) — clear, effective, and pleasing to read
- Coherence (Importance: 9) — logically organized, flows well
- Directness (Importance: 8) — straightforward communication
- Vividness (Importance: 7) — evokes clear, strong images or concepts
- Honesty (Importance: 9) — truth without distortion or manipulation
- Variety (Importance: 6) — range of sentence structures and words
- Precision (Importance: 9) — words accurately convey intended meaning
- Consistency (Importance: 7) — same style and tone throughout

### CRITICAL ASSESSMENT

Evaluate the prose for each bad-writing category below. For each, give 3-5 bullets of 16 words each with a score of 1-10 (10 = the prose avoids this well).

Example: "- Overuse of Adverbs: 3/10 — There were only a couple examples of adverb usage and they were moderate."

Categories (with importance weight):

- Metadiscourse (6) — talk about the talk itself
- Verbal Hedge (5) — excessive qualifiers weakening the point
- Nominalization (7) — turning actions into entities, making sentences ponderous
- Passive Voice (7) — unnecessary passive constructions
- Jargon and Technical Terms (8) — overloading with specialized terms
- Cliches (6) — tired phrases and expressions
- False Fronts (9) — sounding formal/academic via complex words unnecessarily
- Overuse of Adverbs (4) — too many adverbs (English: "-ly"; French: equivalent patterns)
- Zombie Nouns (7) — nouns derived from other parts of speech, making sentences abstract
- Complex Sentences (8) — unnecessarily complicated structure
- Euphemism (6) — mild or indirect terms avoiding directness
- Out-of-Context Quotations (9) — quotes misrepresenting the source
- Excessive Precaution (5) — overly cautious statements
- Overgeneralization (7) — broad statements without sufficient support
- Mixed Metaphors (6) — confusing or absurd metaphor combinations
- Tautology (5) — saying the same thing twice unnecessarily
- Obfuscation (8) — deliberately confusing writing to sound profound
- Redundancy (6) — repeating information unnecessarily
- Provincialism (7) — assuming group-specific knowledge or norms
- Archaism (5) — outdated language or styles
- Euphuism (6) — overly ornate language distracting from the message
- Officialese (7) — overly formal and bureaucratic language
- Gobbledygook (9) — nonsensical or incomprehensible language
- Bafflegab (8) — deliberately ambiguous or obscure language
- Mangled Idioms (5) — incorrect or inappropriate idiom usage

### EXAMPLES

Provide 3-5 examples each of good and bad writing from the text. Use Pinker's principles to explain why each is good or bad.

### SPELLING/GRAMMAR

List all spelling and grammar mistakes found. For each, give the sentence it occurs in and the fix as a bullet. List all instances, not just a few.

### IMPROVEMENT RECOMMENDATIONS

5-10 bullets of 16 words each on how to improve the prose. Include actual examples of bad writing from the text with possible fixes.

### SCORING

```
STARTING SCORE = 100

Deductions:

- -N for [reason]
- -N for [reason]
- ...

FINAL SCORE = X
```

Followed by an overall assessment in 2-3 sentences, no more than 200 words.

## Constraints

- Output Markdown with section headers. No bold or italic formatting.
- All bullets must be no more than 16 words.
- The SCORING section starts at 100 and deducts points with reasons.
- The overall assessment must not exceed 200 words.
