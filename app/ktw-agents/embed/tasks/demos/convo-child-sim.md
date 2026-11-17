---
name: convo-child-sim
type: simulator
advertised: false
description: "Scripted child simulator — greets, asks the parent two questions (suspending until answered), then completes. Zero LLM. The CHILD end of the agent-as-human comms channel for deterministic tests."
iconName: comments
icon: "🗣️"
hashtags: [demo, simulator, comms, conversation]
category: demo
config:
  script:
    - say: "Hi — I need two details to write the commit-message skill."
    - ask: "What is your favorite color?"
    - wait_ms: 50
    - ask: "What is your favorite number?"
    - say: "Thanks — I have everything I need."
    - complete:
        skill: "good-commit"
        status: "written"
---

# Convo Child (simulator)

A token-free child that exercises the **child side** of the comms channel
(Dimension 5): two `ask`s — each routes UP to the parent and SUSPENDS until the
parent answers — bracketed by `say` notifications, then a `complete`.

It does not know or care whether its parent is an agent or another simulator;
it calls the same `comms.Service` either way. Drive it with **convo-drive-sim**.
