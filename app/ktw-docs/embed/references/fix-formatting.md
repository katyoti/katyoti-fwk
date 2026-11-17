# Fix Formatting

Correct heading capitalization and section structure in a French-language Markdown document.

## Purpose

Standardize headings and formatting in French Markdown documents according to French typographic conventions and proper Markdown structure.

## Scope

- **Input**: a French `.md` file.
- **Allowed tools**: Read, Edit, Glob.
- **argument-hint**: `[file-path]`

## Process

### Step 1 — Read the document

1. Read the target document in full.
2. Identify all headings (`#`, `##`, `###`, etc.) and section separators (`---`).

### Step 2 — Apply formatting rules

Apply **every** rule below. Never skip one.

#### Rule A — French title capitalization

- Only capitalize the **first word** of each heading and any **proper nouns**.
- Do **not** capitalize each word (no Title Case).
- Examples:
  - DO: `# Gestion des identifiants utilisateur`
  - DO NOT: `# Gestion Des Identifiants Utilisateur`
  - DO: `## Reference a l'API Google Maps`
  - DO NOT: `## Reference A L'Api Google Maps`

#### Rule B — Remove section separators

- Remove all horizontal rules (`---`) used as section separators between sections.
- Do **not** remove `---` used for YAML frontmatter delimiters.

#### Rule C — Page title not numbered

- The first `#` heading in the document is the **page title**.
- It is not a numbered section and must not carry a number.

#### Rule D — No explicit numbering in headings

- Remove any explicit numbers from headings (e.g., `1.`, `2.1`, `A.`).
- Use Markdown heading levels (`#`, `##`, `###`, etc.) to indicate structure.
- Examples:
  - DO: `# Titre`
  - DO NOT: `# 1. Titre`
  - DO: `## Sous-section`
  - DO NOT: `## 2.1 Sous-section`

#### Rule E — Heading level structure

- Sections after the page title begin at level 1 (`#`), not level 0.
- Maintain a logical heading hierarchy: `#` -> `##` -> `###`.
- Do not skip heading levels (e.g., `#` directly to `###`).

### Step 3 — Apply corrections

Use the Edit tool to apply all necessary corrections directly to the document.

### Step 4 — Produce a summary

After applying corrections, produce a brief summary listing:

1. **Total corrections made** (count).
2. **Corrections by rule** — for each rule (A through E), list the number of corrections applied and give 1-2 examples of changes made (showing before -> after).
3. If no corrections were needed for a rule, state it explicitly.
