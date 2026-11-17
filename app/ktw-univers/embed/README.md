# Embedded resources for ktw-univers

This directory hosts files that are compiled into the `ktw-univers` binary
via `go:embed`. v1 ships no embedded resources; the placeholder lets the
binary build cleanly.

Future use cases:
- Bundled default skills for the orchestrator's own help text
- Embedded universe templates for `ktw-univers create --template <name>`
