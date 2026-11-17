# ktw-kb

Knowledge-base pipeline sidecar — sync sources through ktw-sync, extract
them into knowledge bundles, chunk + embed + index in chromem-go, expose
semantic search via KTSP.

## Composes modules

| Module | Role |
|---|---|
| [kb](../../modules/kb/) | Domain logic |
| [framework](../../modules/framework/) | Bootstrap |
| [catalog](../../modules/contributions/) | Manifest |
| [skills](../../modules/skills/) | KB-related skills |
| [scripts](../../modules/scripts/) | Skill scripts |

## Bootstrap

See [cmd/bootstrap.go](cmd/bootstrap.go).

## Storage

Each knowledge base is a project folder; its vector index lives in
`<kb_root>/.index/` (chromem-go in-process).
