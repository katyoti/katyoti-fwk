# ktw-utils

Utility commands for inspecting and exporting ktw-agents session data.

## Commands

### `ktw-utils journal`

Browse or export LLM session journals in a human-readable format.

```bash
# Interactive: pick a session, then browse in TUI
ktw-utils journal

# Export a session to markdown
ktw-utils journal --export

# Open a specific session by ID
ktw-utils journal 2026-03-04T18-21-08_song-writer

# Export a specific session
ktw-utils journal --export 2026-03-04T18-21-08_song-writer
```

**TUI browser controls**: `←`/`→` navigate turns, `↑`/`↓` scroll, `q` quit.

### `ktw-utils version`

Print the version string.

## Session Data

Sessions are stored in `~/.config/ktw-agents/sessions/`. Each session directory contains:

- `llm.jsonl` — LLM request/response events
- `user.jsonl` — user interaction events
- `summary.json` — pre-computed metrics (turns, tokens, duration)
