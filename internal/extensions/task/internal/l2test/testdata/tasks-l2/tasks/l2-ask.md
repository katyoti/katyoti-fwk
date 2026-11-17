---
name: l2-ask
type: agent
description: "L2 harness: an agent that asks the person through the inbox, then completes with the answer. Its model is the harness's fake endpoint."
config:
  capability: l2fake
---

# L2 ask

1. Call `ask-user` with the question "L2 harness: may the run go on?".
2. When the answer comes, call `complete` with a summary that quotes it.
