# Diagram

Reference for creating standalone diagram images via the `diagram` tool with an interactive, iterative workflow.

---

## Purpose

Create standalone diagram images (SVG, PNG, JPG, PDF) rendered by the ktw-docs sidecar's `diagram` tool. Supports 28+ diagram types. Accepts three input modes: no input (fully guided), a text/markdown file as requirements, or an existing `.diagram.txt` definition file for refinement.

## Scope

Input: `$ARGUMENTS` -- a description, a file path (text/markdown), or an existing `.diagram.txt` definition file.

Output: a rendered diagram image file (`<name>.<format>`). The `.diagram.txt` definition file is an optional authoring aid — it preserves intent and source between iterations but is not required by the renderer.

## Rendering

Rendering is a single `diagram` tool call:

```json
{
  "type": "mermaid",
  "source": "graph TD\n    A[Start] --> B[End]",
  "format": "svg"
}
```

Parameters: `type` (diagram type, e.g. `mermaid`, `plantuml`, `graphviz`, `d2`, `c4`, …), `source` (inline diagram source code), `format` (`svg` | `png` | `jpg` | `pdf`, default `svg`). The tool returns the output path. The agent may either pass the source inline or extract it from Zone 3 of a `.diagram.txt` file (see below).

## Definition File Format (optional authoring aid)

The `.diagram.txt` convention is a three-zone file that preserves diagram intent, requirements, and source in one place. It's useful for iteration — the user can edit it and ask for a re-render later. When used, the agent reads Zone 1 for `type` + `format`, reads Zone 3 for the `source` string, and passes them to `diagram`.

Zones are separated by `==========`:

```
type: mermaid
format: svg
==========
User description of requirements goes here.
What the diagram should represent, context, audience, etc.
==========
graph TD
    A[Start] --> B[End]
```

**Zone 1 -- YAML frontmatter** (required fields):
- `type`: Diagram type (mermaid, plantuml, graphviz, d2, vegalite, etc.)
- `format`: Output format (`svg`, `png`, `jpg`, `pdf`). Default: `svg`

**Zone 2 -- User requirements**: Free-text description of what the diagram should represent. Kept for reference and future refinement.

**Zone 3 -- Diagram source**: The actual diagram code sent to Kroki. Format depends on diagram type (text DSL, JSON, XML, etc.).

## Diagram Type Selection and Syntax

**Type and engine selection is a skill, not a table lookup**: load
`ktw-docs:/skill/diagram-create` to go from intention to the right diagram
type — it clarifies what must be shown, picks type + engine, and routes to the
engine's syntax. For Mermaid source it routes to `ktw-docs:/skill/mermaid-syntax`
(per-type references under [diagrams/mermaid/](diagrams/mermaid/)); for
PlantUML/C4 source to `ktw-docs:/skill/plantuml-syntax` (per-type references
under [diagrams/plantuml/](diagrams/plantuml/)).

Family references for the other engines:

| Family | Reference | Diagram types |
|--------|-----------|---------------|
| Flowcharts & Graphs | [flowcharts.md](diagrams/flowcharts.md) | GraphViz, D2 (Mermaid: see the skill above) |
| UML | [uml.md](diagrams/uml.md) | Nomnoml (PlantUML: see the skill above) |
| Architecture | [architecture.md](diagrams/architecture.md) | Structurizr (C4-PlantUML: see the skill above) |
| Data Visualization | [data-visualization.md](diagrams/data-visualization.md) | Vega, Vega-Lite |
| Network & Infrastructure | [network-infra.md](diagrams/network-infra.md) | NwDiag, RackDiag, PacketDiag, BlockDiag, SeqDiag, ActDiag |
| Miscellaneous | [misc-diagrams.md](diagrams/misc-diagrams.md) | BPMN, Excalidraw, Ditaa, SvgBob, Erd, DBML, WaveDrom, Bytefield, WireViz, Pikchr, TikZ, UMLet |

Also reference [diagram-types.md](diagrams/diagram-types.md) for the full
need→type catalog, and [diagram-pitfalls.md](diagrams/diagram-pitfalls.md)
before finalizing any diagram.

## Process

### Step 1 -- Determine input mode

Check `$ARGUMENTS` to determine the input mode:

#### Mode A: No input (full guided process)

Ask the user about their diagram needs using `AskUserQuestion`:

1. **Purpose**: What should the diagram represent? (process flow, architecture, data relationships, network topology, etc.)
2. **Context**: What is this diagram for? (documentation, presentation, technical spec, etc.)
3. **Diagram type preference**: Does the user have a preference, or should one be recommended?

Load `ktw-docs:/skill/diagram-create` to select the best diagram type and engine based on the user's needs (the full need→type catalog is [diagram-types.md](diagrams/diagram-types.md)). If multiple types could work, use `AskUserQuestion` to present 2-3 options with brief descriptions of trade-offs.

Then gather the specific content: entities, relationships, steps, data, etc.

#### Mode B: Text/markdown file as input

1. Read the input file.
2. Analyze its content to identify what should be diagrammed.
3. Propose a diagram type and a rough structure to the user. Use `AskUserQuestion` if multiple approaches are viable.
4. Proceed to diagram creation.

#### Mode C: Existing `.diagram.txt` definition file

1. Read and parse the definition file (all 3 zones).
2. Show the user the current state: diagram type, requirements, and source.
3. Ask what they want to change: requirements, diagram type, or refine the diagram source.
4. Apply changes and proceed to rendering.

### Step 2 -- Propose the diagram

Before writing the definition file:

1. Read the appropriate diagram family reference from the dispatch table above for the selected diagram type's syntax.
2. Draft the diagram source code.
3. Present the proposed diagram to the user:
   - Show the diagram source in a fenced code block.
   - Explain the structure and key elements.
   - Ask for validation using `AskUserQuestion` with options like:
     - "Looks good, generate the image"
     - "I want to adjust some elements"
     - "Try a different diagram type"
     - "Start over with different requirements"
4. If the user wants adjustments, iterate until satisfied.

### Step 3 -- (Optional) Write the definition file

If the user wants an iteration artifact on disk, once they validate the diagram:

1. Choose a descriptive filename: `<name>.diagram.txt` (e.g., `auth-flow.diagram.txt`, `system-architecture.diagram.txt`).
2. Write the definition file with all 3 zones.
3. Place it in the same directory as the input file, or the current working directory.

This step is optional. If the user just wants the image, skip straight to Step 4 with the source inline.

### Step 4 -- Render the image

Call the `diagram` tool:

```json
{
  "type": "<type from Zone 1 or user choice>",
  "source": "<source from Zone 3 or inline>",
  "format": "<format from Zone 1 or svg>"
}
```

**Before / After**

| Before (legacy) | After (this skill) |
|---|---|
| Write `auth-flow.diagram.txt` with 3 zones, then `python scripts/transform.py auth-flow.diagram.txt` | Call `diagram({type:"mermaid", source:"graph TD\n...", format:"svg"})` — the `.diagram.txt` file is optional |

The tool returns a JSON object with `outputPath` and `diagramType`. Write the returned `outputPath` to the user.

If rendering fails, read the error message, fix the source (or the `.diagram.txt` Zone 3 if you wrote one), and retry.

### Step 5 -- Review and iterate

After rendering:

1. Inform the user the image has been generated and where it is saved.
2. Ask if they want to refine the diagram using `AskUserQuestion`:
   - "I'm satisfied with the result"
   - "I want to adjust the diagram"
   - "Change the output format"
   - "Try a different diagram type"
3. If refining, update the source (or the `.diagram.txt` zones 1 and/or 3) and re-render.

## Environment

- The Kroki endpoint is configured in the ktw-docs sidecar. Defaults to `https://kroki.io`; a local Kroki can be wired via sidecar configuration. No skill-level environment setup is required.
- No Python / pyyaml / pandocfilters / host-pandoc dependencies. All rendering runs server-side in the sidecar.

## Constraints

- Always read the relevant diagram family reference before writing diagram source code.
- Always read [diagram-pitfalls.md](diagrams/diagram-pitfalls.md) before finalizing any diagram.
- When using `.diagram.txt` as an authoring aid, it must contain exactly 3 zones separated by `==========`, and Zone 1 must include both `type` and `format` YAML fields.
- Use `AskUserQuestion` for user interaction during the guided workflow.
