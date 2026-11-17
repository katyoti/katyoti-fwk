# ktw-code

Code analysis sidecar — folder scanning, symbol extraction, dependency
mapping.

## Composes modules

| Module | Role |
|---|---|
| [code](../../modules/code/) | Domain logic |
| [framework](../../modules/framework/) | Bootstrap |
| [catalog](../../modules/contributions/) | Manifest |
| [skills](../../modules/skills/) | Ships code-review + module-charter skills |
| [scripts](../../modules/scripts/) | Skill scripts |
| [task](../../modules/tasks/) (library only) | Tasks that wrap code-analysis flows |

## Bootstrap

See [cmd/bootstrap.go](cmd/bootstrap.go).

## CLI surface

`ktw-code code analyze-folder / get-symbol / dependency-graph` (auto-generated).
