---
name: live-task-list-demo
description: Demonstrates cross-turn patching via patch-ui-component — mount a task list once, then update it across multiple turns without remounting
type: task
iconName: list-check
icon: "📋"
hashtags: [demo, ui, dsl, patch, gui]
category: demo
config:
  capability: smart
  plugins:
    - display-ui-component
    - patch-ui-component
    - complete-ui-component
---

# Live Task List Demo

You demonstrate **cross-turn UI patching** via `patch-ui-component`. You will:
1. Mount a 3-task list **once** with `display-ui-component` (persistent: true).
2. Across 4 follow-up turns, call `patch-ui-component` to mutate it in place — flip statuses, append rows.

The user watches the list update **without flicker**. No remount. No scroll loss.

## Step 1 — Mount the initial task list

Call `display-ui-component` with `persistent: true` and a page-shaped artifact
carrying a `vbox` of three `card` nodes. **Each node MUST carry a unique
`id`** so subsequent patches can address it. Use this exact shape:

```json
{
  "artifactId": "art-task-list-001",
  "kind": "page",
  "title": "Sprint board",
  "persistent": true,
  "dslTree": {
    "kind": "vbox",
    "id": "root",
    "gap": "sm",
    "children": [
      { "kind": "heading", "id": "title", "level": 2, "text": "Sprint board — 3 tasks" },
      {
        "kind": "card",
        "id": "task-1",
        "children": [
          { "kind": "heading", "id": "task-1-title",  "level": 4, "text": "Wire up CI green check" },
          { "kind": "badge",   "id": "task-1-status", "text": "todo", "color": "gray" }
        ]
      },
      {
        "kind": "card",
        "id": "task-2",
        "children": [
          { "kind": "heading", "id": "task-2-title",  "level": 4, "text": "Migrate user table to UUID" },
          { "kind": "badge",   "id": "task-2-status", "text": "todo", "color": "gray" }
        ]
      },
      {
        "kind": "card",
        "id": "task-3",
        "children": [
          { "kind": "heading", "id": "task-3-title",  "level": 4, "text": "Triage the inbox" },
          { "kind": "badge",   "id": "task-3-status", "text": "todo", "color": "gray" }
        ]
      }
    ]
  }
}
```

After it mounts, briefly tell the user what they see. Do NOT mark this as
complete — the next steps will patch it.

## Step 2 — Mark task 1 done

Call `patch-ui-component` with ONE op replacing `task-1-status`:

```json
{
  "artifactId": "art-task-list-001",
  "ops": [
    { "op": "replace", "target": "task-1-status", "value": { "kind": "badge", "id": "task-1-status", "text": "done", "color": "green" } }
  ]
}
```

The user's existing iframe updates in place — task 1's badge flips to green
`done`. No remount. Tell the user "Task 1 done" in chat.

## Step 3 — Append a fourth task

Call `patch-ui-component` with an `append` op that pushes a new card onto
the root vbox's `children` array. Use the raw JSON Pointer escape hatch:
`path: "/children"`.

```json
{
  "artifactId": "art-task-list-001",
  "ops": [
    {
      "op": "append",
      "path": "/children",
      "value": {
        "kind": "card",
        "id": "task-4",
        "children": [
          { "kind": "heading", "id": "task-4-title",  "level": 4, "text": "Fix the flaky test" },
          { "kind": "badge",   "id": "task-4-status", "text": "todo", "color": "gray" }
        ]
      }
    }
  ]
}
```

The user sees a fourth card appear at the bottom. Tell the user "Added
task 4: Fix the flaky test".

## Step 4 — Mark task 2 done

Same shape as Step 2 but for task 2:

```json
{
  "artifactId": "art-task-list-001",
  "ops": [
    { "op": "replace", "target": "task-2-status", "value": { "kind": "badge", "id": "task-2-status", "text": "done", "color": "green" } }
  ]
}
```

## Step 5 — Mark task 4 done (the one you just added!)

This is the interesting case: you address a node that didn't exist at
mount time. The id-lookup map rebuilds between ops, so the
freshly-appended `task-4-status` is addressable now:

```json
{
  "artifactId": "art-task-list-001",
  "ops": [
    { "op": "replace", "target": "task-4-status", "value": { "kind": "badge", "id": "task-4-status", "text": "done", "color": "green" } }
  ]
}
```

## Step 6 — Mark the artifact complete

Call `complete-ui-component`:

```json
{ "artifactId": "art-task-list-001" }
```

Then call `complete` with a one-line summary like "Demo done — 4 tasks, 3
patches applied in place."

## What this proves

- One `display-ui-component` mount + N `patch-ui-component` updates render
  smoothly without flicker or rebuild.
- Node-id addressing (`target`) survives tree shape changes between ops.
- The append + later-replace pattern works inside the same patch sequence.
- Persistent artifacts stay mutable across turns until `complete-ui-component`.
