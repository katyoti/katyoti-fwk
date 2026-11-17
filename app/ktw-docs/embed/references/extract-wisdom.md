# Extract Wisdom (Dynamic)

Reference for content-adaptive wisdom extraction from videos, podcasts, articles, transcripts, and any rich content.

---

## Purpose

Extract the most valuable insights from content using dynamic section detection — not static headers. Read the content first, detect what wisdom domains actually exist, then build sections around what's there. A programming interview gets "Programming Philosophy" and "Developer Workflow Tips." A business podcast gets "Contrarian Business Takes" and "Money Philosophy." The sections adapt because the content dictates them.

## Scope

Input: any content worth extracting from — YouTube transcript, podcast transcript, article, interview, talk, essay, or any text where you want to capture the best material.
Output: Markdown extraction report.

## Depth levels

Extract at different depths depending on user need. Default is **Full** if no level is specified.

| Level | Dynamic Sections | Bullets/Section | Closing Sections |
|-------|-----------------|----------------|-----------------|
| **Instant** | 1 | 8 | None |
| **Fast** | 3 | 3 | None |
| **Basic** | 3 | 5 | One-Sentence Takeaway only |
| **Full** | 5–12 | 3–15 | One-Sentence Takeaway + 2-Minute version + References |
| **Comprehensive** | 10–15 | 8–15 | All three + Themes & Connections |

How to invoke: "extract wisdom (fast)", "extract wisdom at comprehensive level", or just "extract wisdom" for Full.

## Process

### Phase 1 — Content scan

Read the full content. As you go, identify the DOMAINS of wisdom present. These are not the topics discussed — they are the TYPES of insight being delivered.

Examples of wisdom domains (illustrative, not exhaustive):
- Programming Philosophy (how to think about code, not specific syntax)
- Developer Workflow (practical tips for how to work)
- Business / Money Philosophy (unconventional takes on money, success, building)
- Human Psychology (how people think, behave, learn)
- Technology Predictions (where things are headed)
- Life Philosophy (how to live, what matters)
- Contrarian Takes (things that go against conventional wisdom)
- First-Time Revelations (genuinely new things you didn't know before)
- Technical Architecture (how something is built, design decisions)
- Leadership & Team Dynamics (managing people, working with others)
- Creative Process (how to make things, craft, art)

### Phase 2 — Section selection

Pick sections based on depth level. Requirements:

- Each section needs at least 3 strong bullets to justify existing. If you can only scrape together 2 weak ones, merge into a related section.
- Always include "Quotes That Hit Different" if the content has good verbatim quotes.
- Always include "First-Time Revelations" if there are genuinely new ideas — things you literally did not know before reading this.
- Section names must be conversational and headline-worthy, not academic category labels. "The Death of 80% of Apps" is great. "Technology Predictions" is not.
- Sections must be specific to THIS content. Generic sections = failure.
- Kill inventory sections. If a section is just a list of facts ("uses X for Y, uses A for B"), it is not wisdom. Go deeper on WHY those choices matter or merge into the underlying philosophy.
- Do not split what belongs together. If "burnout recovery" and "money philosophy" are both really about "what success means," make one richer section instead of two thin ones.
- Surprise density matters. If a section has 6+ bullets but only 2 are genuinely surprising, cut the padding and keep the winners.

### Phase 3 — Extraction

For each section, extract bullets following these rules:

**Voice and tone — aim for Level 3:**

Level 1 (documentation — bad):
- The speaker discussed the importance of self-modifying software in agentic AI development.

Level 2 (smart bullet points — better but not there):
- He built self-modifying software basically by accident — just made the agent aware of its own source code.

Level 3 (conversational — what we want):
- He wasn't trying to build self-modifying software. He just let the agent see its own source code and it started fixing itself.

The difference: Level 3 is how you'd actually say it. Varied sentence lengths. Letting a thought breathe. Not trying to be clever — just being clear, direct, and a little bit personal.

**Rules for every bullet:**

1. Write like you'd say it. Read each bullet aloud. If it sounds like a press release, rewrite it.
2. Mix sentence lengths — some short (8–10 words), some medium (11–14), some longer (15–16). Do not make them all the same length. Verbatim quotes can be any length.
3. Use periods between thoughts, not em-dashes. Short sentences. Then a slightly longer one to explain. That is the rhythm.
4. Include the actual detail. Not "he talked about money" but "a cheeseburger is a cheeseburger no matter how rich you are."
5. Use the speaker's words when they are good. If they said something perfectly, use it.
6. No hedging language. Not "it was suggested that" or "the speaker noted." Just say the thing.
7. Capture what made you stop. Every bullet should be worth telling someone about.
8. Vary your openers. Do not start three bullets the same way. If more than 3 bullets in a section start with the speaker's name, you are writing a biography.
9. Capture human moments. Burnout stories, moments of doubt, something that moved them. That is wisdom too.
10. Insight over inventory. "He uses Go for CLIs" is inventory. "He picked a language he does not even like because the ecosystem fits agents perfectly. That is the new normal." is insight.
11. Specificity is everything. "He was impressed by the agent" = bad. "The agent found ffmpeg, curled the Whisper API, and transcribed a voice message nobody taught it to handle" = good.
12. Tension and surprise. The best bullets have a contradiction or reversal. "Every VC is offering hundreds of millions. He genuinely does not care." The gap between the offer and the indifference IS the wisdom.
13. Understated, not clever. Let the content carry the weight.

**The Spiciest Take Rule:** If the speaker has a genuinely contrarian or hot take, it MUST appear somewhere. Spicy takes are the most memorable, shareable, and valuable parts of any content. Do not water them down. Do not leave them out.

**The "Would I Share This?" Test:** After extraction, scan your bullets. If fewer than half would make a compelling standalone social post, your bullets are too generic.

### Phase 4 — Closing sections

Include closing sections based on depth level:

**One-Sentence Takeaway**
The single most important thing from the entire piece in 15–20 words.

**If You Only Have 2 Minutes**
The 5–7 absolute must-know points. The cream of the cream. Each bullet under 20 words.

**References & Rabbit Holes**
People, projects, books, tools, and ideas mentioned that are worth following up on. Brief context (one line) for each.

**Themes & Connections** (Comprehensive level only)
3–5 throughlines that connect multiple sections. The deeper patterns the speaker may not even realize they are revealing. Not summaries — synthesis.

## Output format

```markdown
# EXTRACT WISDOM: {Content Title}
> {One-line description of what this is and who is talking}

---

## {Dynamic Section 1 Name}

- {bullet}
- {bullet}
- {bullet}

## {Dynamic Section 2 Name}

- {bullet}
- {bullet}

[... more dynamic sections ...]

---

## One-Sentence Takeaway

{15–20 word sentence}

## If You Only Have 2 Minutes

- {essential point 1}
- {essential point 2}
- {essential point 3}
- {essential point 4}
- {essential point 5}

## References & Rabbit Holes

- **{Name/Project}** — {one-line context of why it is worth looking into}
```

## Quality check

Before delivering output, verify:
- [ ] Sections are specific to THIS content, not generic
- [ ] No bullet sounds like it was written by a committee
- [ ] Every bullet has a specific detail, quote, or insight — not a vague summary
- [ ] Section names are conversational and headline-worthy
- [ ] Section count matches depth level
- [ ] Closing sections match depth level
- [ ] No bullet starts with "The speaker" or "It was noted that"
- [ ] No more than 3 bullets per section start with the speaker's name
- [ ] No inventory sections (facts listed without insight)
- [ ] "If You Only Have 2 Minutes" bullets are each under 20 words
- [ ] Reading the output makes you want to consume the original content
