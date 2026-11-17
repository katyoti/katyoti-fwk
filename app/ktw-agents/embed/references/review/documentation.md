# Review guide — documentation

**What it answers:** does the module's documentation say what is true, to the
people who read it, completely and in one place?

## Read

The module's pages (`modules/<m>/*.md`), its README if any, the goals and
features (their descriptions and acceptance criteria), the doc comments of
its code packages' exported API, and the change-log entries naming its
requests (`docs/confluence-tew/changelog/`).

## Judge

1. **Inventory** — which pages exist (concepts, tools, a guide), what each
   covers; a page with no reader is a finding.
2. **Accuracy** — cross-check each claim a page makes with the code or the
   objects: a type, a field, a verb, a panel that no longer exists or behaves
   otherwise is a finding (severity by how much a reader is misled).
3. **Completeness** — every user-visible behaviour of the module (its
   features) is described somewhere; every exported API is commented.
4. **Form** — pages state what IS (no history, dates, "used to", request
   numbers), one place per subject, links that resolve, consistent terms.
5. **Audience** — user pages written for users, developer pages for
   developers.

## Rate

`critical` when a page misleads a reader into a wrong action or a core
behaviour is undocumented; `needs-attention` for gaps or stale statements;
`good` otherwise. Score: 100 minus 15 per high finding, 5 per medium, 1 per
low, never below 0.

## Report

The inventory as a table (page, covers, assessment), then the accuracy
issues, the gaps, the form issues, the strengths, what to do next.
