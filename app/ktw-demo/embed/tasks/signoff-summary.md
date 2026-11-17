---
name: signoff-summary
title: Sum up the sign-off
type: simulator
kind: object
types: [signoff-item]
description: "Sum up the sign-off of the current sign-off item — a no-LLM demo task offered on every sign-off item's action bar."
iconName: list-check
icon: "🧾"
hashtags: [demo, signoff, simulator]
category: demo
config:
  duration_ms: 0
  output:
    status: signoff-summary-done
    note: "kind:object types=[signoff-item] — offered on a sign-off item's action bar."
---

# Sign-off summary (simulator)

A `kind: object` task targeting sign-off items: it appears among the actions of
every sign-off item, the item threaded as its target. It runs with no LLM and
completes with `signoff-summary-done`.
