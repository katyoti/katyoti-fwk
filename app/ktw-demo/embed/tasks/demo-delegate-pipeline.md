---
name: demo-delegate-pipeline
type: workflow
description: "Sequential agent pipeline (researcher → analyst → writer)"
iconName: arrow-right-circle
icon: "➡️"
hashtags: [demo, delegate]
category: demo
config:
  steps:
    - name: researcher
      task:
        type: task
        capability: fast
        plugins: [standard]
        prompt: |
          You are the **researcher** in a 3-stage pipeline.

          Begin by saying "starting researcher".

          Your task: name 3 colors. Reply with a single line listing them, comma-separated.

          End by saying "finished researcher", then call `complete` with your list of colors.

    - name: analyst
      task:
        type: task
        capability: fast
        plugins: [standard]
        prompt: |
          You are the **analyst** in a 3-stage pipeline.

          Begin by saying "starting analyst".

          The researcher gave you these colors: {{ .colors }}

          Categorize each color as warm, cool, or neutral. Output one line per color.

          End by saying "finished analyst", then call `complete` with your categorization.
      input:
        colors: "${results.researcher}"

    - name: writer
      task:
        type: task
        capability: fast
        plugins: [standard]
        prompt: |
          You are the **writer** in a 3-stage pipeline.

          Begin by saying "starting writer".

          The analyst gave you this categorization: {{ .categorization }}

          Compose ONE short sentence that uses all the colors and references their categorization.

          End by saying "finished writer", then call `complete` with your sentence.
      input:
        categorization: "${results.analyst}"

    - kind: return
      value: "${results.writer}"
---

# Demo: delegate-pipeline

**Pattern**: workflow with three sequential agent steps. Each step's output feeds
the next via `input` bindings. No branching, no parallelism — the simplest
possible illustration of a deterministic agent pipeline.

**What to watch in the journal / GUI Team tab**:
- Three child sessions appear sequentially: `researcher` → `analyst` → `writer`
- Each agent emits "starting <role>" / "finished <role>" status lines
- Agent N's `complete` output appears as input to agent N+1

**Cost**: ~5K tokens total (~$0.005 with `fast-best`). Trivial prompts on purpose
— this demo proves the topology, not LLM intelligence.
