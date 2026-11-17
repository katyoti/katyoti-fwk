# ktw-docs

Document + diagram transformation sidecar. Renders structured payloads
into Markdown, HTML, SVG, DrawIO, GraphML, PDF; publishes to Confluence.

## Composes modules

| Module | Role |
|---|---|
| [doc-engine](../../modules/doc-engine/) | Domain logic (ELK layout + multi-format render + Confluence) |
| [framework](../../modules/framework/) | Bootstrap |
| [catalog](../../modules/contributions/) | Manifest hydration |
| [skills](../../modules/skills/) | Ships many skills under `embed/skills/` |
| [scripts](../../modules/scripts/) | Skill scripts |

## Bootstrap

See [cmd/bootstrap.go](cmd/bootstrap.go).

## External dependencies

- `pandoc` (required) — for format conversion.
- ELK runtime (bundled).
