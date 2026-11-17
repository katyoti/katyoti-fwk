# Explain Docs

Reference for transforming tool documentation into a clear, structured guide.

---

## Purpose

Turn tool documentation or instructions into a clear, structured guide with overview, common syntax, use cases, and key options.

## Scope

Input: tool documentation, man pages, CLI help output, README content, product instructions, or any description of how to use a tool.

Interpret the input as tool documentation regardless of its format.

## Process

### Step 1 — Analyze the documentation

Read the input fully. Identify the tool's purpose, most common usage patterns, key options, and common use cases.

### Step 2 — Produce the structured guide

Output in Markdown with the following sections:

### OVERVIEW

- What It Does: (25-word explanation of what the tool does)
- Why People Use It: (25-word explanation of why the tool is useful)

### HOW TO USE IT

- Most Common Syntax: (the most common usage syntax, as a code snippet)

### COMMON USE CASES

List common use cases using this format:

```
For Getting the Current Time: `time --get-current`
For Determining One's Birth Day: `time --get-birth-day`
```

Draw from both the input documentation and general knowledge of the tool.

### MOST IMPORTANT AND USED OPTIONS AND FEATURES

List the most important options, switches, flags, and features from the documentation. For each one, describe how and why it is useful.

## Constraints

- What It Does and Why People Use It must each be no more than 25 words.
- Common use cases must include the command or syntax as a code snippet.
- Options and features descriptions should explain how and why, not just what.
