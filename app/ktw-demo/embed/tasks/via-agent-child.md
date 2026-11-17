---
name: via-agent-child
advertised: false
description: Demo child — asks the user two questions via ask-user, then completes echoing the answers. The CALLER decides the com mode (e.g. agent_as_human); this task knows nothing about bubbling.
type: agent
iconName: robot
icon: "🧒"
hashtags: [demo, test, ask-user]
category: demo
config:
  capability: fast
  mode: worker
  plugins:
    - standard
---

# Via-Agent Child

Gather two facts from the user and echo them back.

1. Call **ask-user**: "What is your favorite color?"
2. Call **ask-user**: "What is your favorite number?"
3. With BOTH answers, call **complete** with summary exactly:
   `Color=<color>, Number=<number>`.

Obtain each value via **ask-user** — never invent them. One question per call.
