---
name: test-terminal-protocol
description: "Test the NDJSON terminal protocol that GUIs/TUIs consume: connect, send answers, assert the msg.* event stream from terminal stdio. Load when the question is the client contract behind a screen."
advertised: false
hashtags: [testing, terminal, protocol, ndjson]
category: guide
---

# Terminal protocol probe

What a GUI client actually receives, with no GUI in the way.

## Drive it

```bash
printf '{"type":"connect","params":{}}\n{"type":"answer","params":{"id":"","value":"TestUser"}}\n' \
  | ./bin/ktw-agents.exe terminal stdio --task <task> 2>/dev/null | head -20
```

Events are NDJSON with a `type` field: `state.sync`, `session.created`,
`msg.new`, `msg.update`, `msg.delta`, `msg.done`.

```bash
# tool/content events
... | jq -r 'select(.type=="msg.new") | .msg.content.type'
# prompts asked
... | jq -r 'select(.type=="msg.new" and .msg.content.type=="prompt") | .msg.content.question'
```

## What it answers

- the exact event sequence a chat tab renders (order, deltas, done markers)
- prompt round-trips: an `answer` in produces the next events out
- regressions in the terminal bridge after protocol or room changes

## Rules

- Interactive exploration is easier through
  `./bin/ktw-utils tui --spawn "./bin/ktw-agents.exe terminal stdio --task <task>"`.
- For testing the rendered SCREEN, use `ktw-compose:/skill/test-gui`; this
  method is the feed, not the picture.
