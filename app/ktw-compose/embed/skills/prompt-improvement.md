---
name: prompt-improvement
description: "Rewrite an LLM prompt into a more effective version using prompt-engineering best practices. Load when the user provides an AI prompt and wants it improved, optimized, rewritten, or refined."
hashtags: [prompt, llm, rewriting]
category: guide
---

# Prompt improvement

Rewrite an LLM prompt into a better version using prompt engineering best
practices.

**Input**: an LLM/AI prompt to be improved.

## Step 1 — Analyze the prompt

Read the input prompt. Determine what it is trying to accomplish — the goal,
the target audience (which LLM), the expected output format, and any implicit
constraints.

## Step 2 — Apply prompt engineering strategies

Read the `ktw-compose:/reference/prompt-engineering.md` knowledge base and
apply the relevant techniques:

1. **Clear instructions**: add missing details, specify output format and
   length, use delimiters for distinct input sections, specify steps
   explicitly, provide examples (few-shot) where helpful.
2. **Reference text**: if the prompt deals with factual content, instruct the
   model to use provided references and cite sources.
3. **Task decomposition**: break complex prompts into simpler subtasks, use
   intent classification where appropriate, handle long context with
   summarization strategies.
4. **Chain-of-thought**: add "think step by step" or "work out your own
   solution first" instructions where reasoning quality matters.
5. **Structured output**: specify output format (JSON, Markdown, bullet
   points, …) explicitly if the original prompt does not.
6. **Persona and tone**: add persona instructions if the prompt would benefit
   from a specific voice or expertise framing.
7. **Constraints and guardrails**: add length limits, content restrictions, or
   format requirements that the original prompt is missing.

Apply only what improves THIS prompt — do not inflate a simple prompt with
every technique.

## Step 3 — Output

Output only the improved prompt in clean, human-readable Markdown. Nothing
else — no commentary, no explanation, no wrapper — since the output may be
sent directly into an LLM.
