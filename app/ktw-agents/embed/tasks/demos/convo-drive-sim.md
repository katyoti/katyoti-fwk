---
name: convo-drive-sim
type: simulator
description: "Scripted PARENT simulator — delegates convo-child-sim (agent_as_human) and answers its bubbled questions with scripted answers, then completes. Zero LLM. THE deterministic fixture for the parent↔child comms channel + the --console renderer."
iconName: masks-theater
icon: "🎭"
hashtags: [demo, simulator, comms, conversation, drive]
category: demo
config:
  script:
    - say: "Driving convo-child-sim to build the commit-message skill."
    - drive:
        task: convo-child-sim
        finish: self
        answers:
          - "blue"
          - "7"
    - complete:
        ok: true
        note: "child driven to completion via scripted answers — no LLM"
---

# Convo Drive (simulator)

The **parent side** of the comms channel, scripted: delegate `convo-child-sim`
in `agent_as_human` mode and answer its questions (`blue`, `7`) with zero LLM.
The `drive` step runs the real await→answer loop — the same `comms.Service` an
agent parent uses — so this fixture validates the whole channel deterministically.

Run it with `--console` to watch the parent and child panes converse:

```
ktw-agents run convo-drive-sim --console
```
