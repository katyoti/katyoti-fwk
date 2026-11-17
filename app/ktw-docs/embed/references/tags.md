# Tags

Reference for extracting tags from text content for mind mapping and categorization.

---

## Purpose

Extract tags from text content for mind mapping and categorization tools. Identifies at least 5 subjects or ideas as lowercase tags from the input.

## Scope

Input: any text content (article, notes, document, etc.).

## Process

### Step 1 -- Identify tags

Read the input fully. Identify at least 5 subjects or ideas to use as tags. If an author or existing tags are present in the input, include them as tags.

### Step 2 -- Output

Output a single line of tags separated by spaces.

## Output Format

A single line of space-separated tags.

Example: `machine_learning python data_science neural_networks deep_learning`

## Constraints

- Minimum 5 tags.
- All tags in lowercase.
- No spaces within tags -- replace spaces with underscores.
- No duplicate tags.
- Include author or existing tags from the input if present.
- Output must be a single line.
