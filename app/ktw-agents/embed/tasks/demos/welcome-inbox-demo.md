---
name: welcome-inbox-demo
title: Welcome question
type: workflow
description: "Files one question in your inbox for the welcome tour, with no LLM: answer it in the Inbox to see how a task asks you something"
iconName: inbox
icon: "📥"
hashtags: [guide, welcome, inbox, demo]
category: demo
config:
  steps:
    - kind: call
      name: file
      uri: ktw-agents:inbox/file
      input:
        title: Welcome to KaTyoti
        description: "This question comes from the welcome tour. Tasks ask you things here when they need a decision; answer it with anything, then press Send."
        requested_by: welcome-inbox-demo
        urgency: low
---
Files the welcome tour's inbox question. It runs no model: one call to the
inbox's own `file` method, so the welcome tour works without an API key.
