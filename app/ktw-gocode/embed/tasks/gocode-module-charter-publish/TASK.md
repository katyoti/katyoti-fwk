---
name: gocode-module-charter-publish
type: agent
description: >-
  Publish the module charter + manifest projections under the
  module's doc_folder. Items remain canonical; the published JSON
  is byte-stable.
iconName: file-text
icon: "📄"
hashtags: [gocode, charter, manifest, publish, module]
category: gocode

kind: object
types: [module]

config:
  capability: fast
  maxTurns: 3
---

# gocode • Charter publish

The user wants to refresh the module's charter and manifest JSON
projections under `<module>/doc_folder/`.

## What to do

1. Read the module ID from `Runtime.CurrentNode` (or `_target`).
2. Call the code service:
   - tool name: `manifest-write`
   - argument: `{ "module_id": "<module-uuid>" }`
3. Call the code service:
   - tool name: `charter-publish`
   - argument: `{ "module_id": "<module-uuid>" }`
4. Surface the two output paths (manifest.json, charter.md) to the
   user.
5. Call `complete` with a one-line summary
   (`charter+manifest published for <module-name>`).

## Notes

- The projections are byte-stable — re-running on the same items
  produces identical JSON (canonical ordering, deterministic).
- Items in CWM stay canonical; the published files are derived
  artefacts — never hand-edit them.
