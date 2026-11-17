# Red Team Architecture Review

## Purpose

Adversarially stress-test an architectural decision, design proposal, ADR (Architecture Decision Record), or significant technical choice found in the codebase. Complements `architecture.md` (which evaluates existing structure) by attacking the *reasoning* behind decisions — decomposing claims into atomic parts, challenging assumptions from multiple expert perspectives, producing a steelman of the best case, and a counter-argument that targets the weakest link.

## Scope

Input: an architectural decision, design document, ADR, significant code comment explaining a technical choice, README architectural section, or any artifact where a technical direction is argued for.
Allowed tools: Read, Grep, Glob.
Output: structured Markdown report with decomposition, steelman, and counter-argument.

## When to use

- A PR contains a significant architectural change with a justification
- An ADR or design doc exists in the repo making a case for a technical direction
- The user asks to stress-test a proposed architecture or technical decision
- `architecture.md` identifies a questionable design decision worth challenging in depth

## Process

### Phase 1 — Identify the target

Read the input. Extract:

1. **Central thesis**: the single claim being made (e.g., "we should use microservices", "this caching layer will solve our performance problem", "event sourcing fits our domain")
2. **Supporting claims**: the reasons given to justify it (numbered list)
3. **Implicit assumptions**: what must be true for this to work, even if not stated
4. **Logical chain**: how the argument flows from premises to conclusion

State the central thesis in one sentence in your own words.

### Phase 2 — Decompose into atomic claims

Break the argument into discrete, individually attackable claims. Each claim must be:
- Self-contained (understandable without the others)
- Specific (not vague)
- Falsifiable (a competent critic could challenge it)

Aim for 8–16 claims depending on argument complexity.

```
CLAIM 1: [atomic claim]
CLAIM 2: [atomic claim]
...
```

### Phase 3 — Multi-perspective analysis

Analyse the claims from four expert perspectives simultaneously. For each perspective, identify the **strongest point for** the argument and the **strongest point against**, with claim references.

#### Perspective 1 — The Systems Engineer
Focuses on: technical correctness, scalability, operational complexity, failure modes, hidden costs.
Key questions: Where does this break at scale? What are the operational consequences? What's the real implementation cost?

#### Perspective 2 — The Architect
Focuses on: structural fit, long-term maintainability, second-order effects, reversibility, how it composes with the rest of the system.
Key questions: Does this fit the existing system? What do we lose? Can we undo this decision in 18 months?

#### Perspective 3 — The Skeptic / Devil's Advocate
Focuses on: unstated assumptions, precedent, reasoning-by-analogy traps, "everyone is doing it" fallacies, what the argument conveniently ignores.
Key questions: What assumption, if false, collapses the entire argument? Has this been tried before and failed?

#### Perspective 4 — The Pragmatist
Focuses on: team capability, delivery risk, simpler alternatives, whether the problem being solved actually exists at the current scale.
Key questions: Is this complexity justified for our actual situation? What's the simpler solution we're not considering?

Output format per perspective:

```
### [Perspective Name]

Strongest point FOR (Claim #X): [2–3 sentences on why this claim is valid]
Strongest point AGAINST (Claim #Y): [2–3 sentences on the flaw]
Overall verdict: [one sentence]
```

### Phase 4 — Constraint classification

Classify every constraint or assumption in the argument:

| Constraint / Assumption | Type | Evidence | Can be challenged? |
|------------------------|------|----------|-------------------|
| [stated constraint] | Hard (physics/reality) / Soft (policy/choice) / Assumption (unvalidated) | [why classified this way] | No / Yes / Possibly |

**Hard constraints** (physics, regulatory, factual) cannot be attacked.
**Soft constraints** (convention, preference, current tooling) can be challenged.
**Assumptions** (unvalidated beliefs treated as facts) are the primary attack targets.

The most revealing critiques expose assumptions classified as Hard that are actually Soft or unvalidated.

### Phase 5 — Steelman

Before attacking, construct the strongest possible version of the argument — not what was written, but what a brilliant advocate would argue.

Output: 6–8 bullets of 12–16 words each representing the best case for the decision.

```
## Steelman: The Best Case For This Decision

1. [12–16 words — strongest opening]
2. [12–16 words — best supporting evidence or precedent]
3. [12–16 words — the real problem being solved]
4. [12–16 words — why the obvious alternatives are worse]
5. [12–16 words — the cost of NOT making this decision]
6. [12–16 words — strongest single reason to proceed]

Validity: [one sentence — the legitimate core concern this decision addresses]
```

### Phase 6 — Counter-argument

Produce the strongest rebuttal that addresses the steelman — not a strawman. The counter-argument must defeat the best version of the argument, not the weakest.

Identify the **single weakest assumption** — the one that, if false, causes the most damage to the argument. Lead with it.

Output: 6–8 bullets of 12–16 words each, escalating to a conclusion.

```
## Counter-Argument: The Case Against

1. [12–16 words — the core assumption that is weakest]
2. [12–16 words — what evidence or precedent contradicts it]
3. [12–16 words — the second-order effect being ignored]
4. [12–16 words — the simpler alternative not considered]
5. [12–16 words — what happens when this decision needs to be reversed]
6. [12–16 words — the knockout: why the steelman's best point still fails]

Verdict: [one sentence — overall assessment of the argument's soundness]
```

### Phase 7 — Recommendations

Based on the analysis, produce concrete recommendations:

- **If the argument holds up**: list the conditions that must remain true and the risks to monitor
- **If the argument is weak**: propose a specific alternative approach
- **If it's mixed**: identify which parts are sound, which need rework, and what evidence would resolve the uncertainty

---

## Output format

```markdown
# Red Team Architecture Review: {decision title}

Date: {today}
Target: {file or section reviewed}

---

## Central Thesis

{one sentence in analyst's own words}

---

## Atomic Claims

1. {claim}
2. {claim}
...

---

## Multi-Perspective Analysis

### Systems Engineer
Strongest FOR (Claim #X): ...
Strongest AGAINST (Claim #Y): ...
Verdict: ...

### Architect
...

### Skeptic
...

### Pragmatist
...

---

## Constraint Classification

| Constraint | Type | Evidence | Challengeable? |
|-----------|------|----------|----------------|
| ... | ... | ... | ... |

---

## Steelman

1. ...
2. ...
...

Validity: ...

---

## Counter-Argument

1. ...
2. ...
...

Verdict: ...

---

## Recommendations

{Concrete next steps based on the analysis}
```

## Rules

- The steelman must represent the best version of the argument — if the author would say "that's a strawman", rewrite it.
- The counter-argument must defeat the steelman, not an easier version.
- Every claim cited in the analysis must reference a specific claim number.
- Do not invent architectural facts not present in the reviewed material.
- Constraint classification is required — the most common failure mode is treating assumptions as hard constraints.
- Recommendations must be concrete: name the alternative, the condition, or the evidence needed — not generic advice.
