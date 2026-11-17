# Argument Analysis

Reference for mapping the logical structure of an argumentative text and evaluating its reasoning quality.

---

## Purpose

Analyze the logical architecture of an argumentative text — essay, position paper, proposal, opinion piece, debate transcript, recommendation memo, or any text built around a central claim. Identifies the thesis, premises, evidence, reasoning chain, and conclusions. Evaluates logical validity, evidence quality, and rhetorical moves. Distinct from prose analysis (which evaluates style) and design review (which evaluates technical decisions) — this evaluates *reasoning*.

## Scope

Input: any text that argues for a position — essay, op-ed, position paper, recommendation, proposal, debate transcript, manifesto, investor memo, policy document, or similar.
Output: Markdown report in the same language as the input text.

## Process

### Step 1 — Extract the argument structure

Read the full text. Identify and map:

1. **Central thesis**: The single main claim the author is trying to establish. State it in one sentence in your own words — not a quote.
2. **Sub-claims**: The major supporting positions the author relies on to reach the thesis.
3. **Premises**: The assumed starting points treated as given, not argued for.
4. **Evidence**: Specific data, examples, citations, anecdotes, or expert opinions used to support sub-claims.
5. **Reasoning chain**: How the author moves from premises + evidence to sub-claims to thesis. Draw this as a logical chain: `[Premise] + [Evidence] → [Sub-claim] → [Thesis]`.
6. **Conclusion**: What the author asks the reader to believe, decide, or do as a result.

### Step 2 — Evaluate argument quality

#### Category A — Logical structure

- A1 -- Validity: If all premises are true, does the conclusion follow logically? Identify any gaps where the conclusion doesn't follow from the stated premises.
- A2 -- Soundness: Are the premises themselves well-supported, or are they asserted without justification?
- A3 -- Logical fallacies: Identify any formal or informal fallacies. For each: name the fallacy, cite the passage, explain why it applies.

Common fallacies to check:
  - Ad hominem — attacking the person, not the argument
  - Straw man — misrepresenting a position to attack it
  - False dichotomy — presenting only two options when more exist
  - Slippery slope — assuming one step inevitably leads to extreme outcomes without justification
  - Appeal to authority — using authority as a substitute for argument
  - Appeal to emotion — using emotional language to bypass reasoning
  - Hasty generalization — drawing a broad conclusion from insufficient examples
  - Circular reasoning — using the conclusion as a premise
  - Post hoc — assuming causation from correlation or sequence
  - Loaded question — embedding an assumption in a question
  - Equivocation — using a word with two meanings as if it has one
  - No true Scotsman — protecting a claim by redefining terms when counterexamples appear

- A4 -- Missing links: Reasoning steps the author skips — places where the argument jumps from A to C without establishing B.
- A5 -- Scope creep: Does the conclusion claim more than the evidence supports? (e.g., evidence supports "X works in context Y" but conclusion claims "X always works")

#### Category B — Evidence quality

- B1 -- Evidence strength: For each piece of supporting evidence, assess its quality:
  - Strong: peer-reviewed data, direct measurement, primary source, large representative sample
  - Moderate: credible secondary source, limited study, plausible expert opinion
  - Weak: anecdote, single example, appeal to common belief, unverified claim
- B2 -- Evidence relevance: Does the evidence actually support the claim it's used for, or is it tangential?
- B3 -- Missing evidence: Key claims asserted without any supporting evidence.
- B4 -- Contradictory evidence ignored: Does the author acknowledge and address counterevidence, or ignore it?
- B5 -- Cherry-picking: Does the selection of evidence appear systematically biased toward supporting the thesis while ignoring contrary data?

#### Category C — Rhetorical moves

- C1 -- Framing: How does the author frame the issue? What is included in the frame, and what is left out? Identify the implicit assumptions embedded in the framing.
- C2 -- Loaded language: Terms that carry emotional or political charge beyond their descriptive meaning. Flag words used to pre-judge rather than describe.
- C3 -- Concessions and steelmanning: Does the author acknowledge the strongest version of the opposing view, or only weak versions? Concessions strengthen an argument; avoiding them is a weakness.
- C4 -- Calls to action and implicit demands: What is the author actually asking of the reader? Is this stated explicitly or embedded implicitly?

#### Category D — Overall assessment

- D1 -- Argument strength: Overall evaluation — Strong / Moderate / Weak — with a 2-3 sentence justification.
- D2 -- Most persuasive element: The single strongest part of the argument.
- D3 -- Most vulnerable element: The single weakest point where the argument is most likely to fail under scrutiny.
- D4 -- Counterarguments not addressed: The 2-3 strongest objections a skeptical reader would raise that the text does not engage with.

### Step 3 — Produce the report

Write the report in the same language as the analyzed text.

Follow this template strictly:

```
# Argument Analysis: {title or first line of text}

Analysis date: {today's date}
Text type: {essay / op-ed / proposal / memo / transcript / other}

---

## Argument map

**Central thesis:** {one sentence in analyst's own words}

**Reasoning chain:**
{[Premise 1] + [Evidence] → [Sub-claim 1]}
{[Premise 2] + [Evidence] → [Sub-claim 2]}
{[Sub-claim 1] + [Sub-claim 2] → [Thesis]}

**Premises (assumed, not argued):**
- {premise}
- {premise}

**Conclusion / Call to action:** {what the author asks the reader to believe or do}

---

## A — Logical structure

### A1 -- Validity
{Does the conclusion follow from the premises? Gaps identified.}

### A2 -- Soundness
{Are the premises themselves supported?}

### A3 -- Logical fallacies
{For each fallacy: name, cited passage, explanation. If none: "No significant fallacies found."}

### A4 -- Missing links
{Reasoning steps skipped.}

### A5 -- Scope creep
{Places where the conclusion overclaims relative to the evidence.}

---

## B — Evidence quality

### B1 -- Evidence strength

| Claim | Evidence Used | Strength | Notes |
|-------|--------------|---------|-------|
| {sub-claim} | {evidence cited} | Strong/Moderate/Weak | {reason} |

### B2 -- Evidence relevance
{Evidence that is tangential to the claim it supports.}

### B3 -- Missing evidence
{Unsupported assertions.}

### B4 -- Contradictory evidence ignored
{Counterevidence the author does not address.}

### B5 -- Cherry-picking
{Assessment of evidence selection bias.}

---

## C — Rhetorical moves

### C1 -- Framing
{How the issue is framed and what the frame excludes.}

### C2 -- Loaded language
{Terms used to pre-judge rather than describe, with citations.}

### C3 -- Concessions and steelmanning
{Whether the author engages with the strongest counterarguments.}

### C4 -- Calls to action
{What the author asks of the reader, explicit and implicit.}

---

## D — Overall assessment

### D1 -- Argument strength
{Strong / Moderate / Weak — with 2-3 sentence justification.}

### D2 -- Most persuasive element
{The single strongest part of the argument.}

### D3 -- Most vulnerable element
{The single weakest point.}

### D4 -- Counterarguments not addressed
1. {Strongest unaddressed objection}
2. {Second objection}
3. {Third objection}

---

## Synthesis table

| # | Category | Finding | Severity |
|---|----------|---------|----------|
| 1 | {A1..D4} | {Short description} | {High/Medium/Low} |

---

## Recommendations

{Numbered list of the most important ways to strengthen the argument — reorder, add evidence, address counterarguments, eliminate fallacies. Ranked by impact.}
```

## Severity definitions

- High: Central thesis does not follow from premises, named logical fallacy, key claim entirely unsupported, contradictory evidence ignored.
- Medium: Missing link in reasoning chain, weak evidence for important sub-claim, scope creep in conclusion, loaded framing on a core term.
- Low: Minor rhetorical move, tangential evidence, stylistic hedging, missing steelman of a minor objection.

## Report rules

- Always cite the specific passage when naming a fallacy or weakness. No abstract accusations.
- The argument map must be completed before the analysis — it anchors everything else.
- Distinguish between "logically invalid" (the reasoning doesn't work) and "empirically unsupported" (the reasoning works but the facts are questionable).
- Do not penalize a text for taking a strong position. Evaluate the quality of the reasoning, not whether you agree with the conclusion.
- Adapt severity labels to the document language (e.g., French: Haute/Moyenne/Basse).
