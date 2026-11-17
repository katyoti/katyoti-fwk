# ktw-docs Test Cases

Sample input files for manual and integration testing of ktw-docs.

## Prerequisites

- `ktw-docs` binary built (`go build ./app/ktw-docs/`)
- For document rendering: `pandoc` and `typst` installed
- For diagram rendering: internet access (Kroki API at kroki.io)

## Diagram Tests (no Pandoc/Typst needed)

```bash
# Render a Mermaid flowchart
ktw-docs diagram mermaid testcases/mermaid-flow.mmd -o /tmp/flow.svg

# Render a PlantUML class diagram
ktw-docs diagram plantuml testcases/plantuml-classes.puml -o /tmp/classes.svg

# Render a Graphviz graph (using "dot" synonym)
ktw-docs diagram dot testcases/graphviz-dag.dot -o /tmp/dag.svg

# Render a D2 diagram
ktw-docs diagram d2 testcases/d2-arch.d2 -o /tmp/arch.svg

# Render as PNG instead of SVG
ktw-docs diagram mermaid testcases/mermaid-flow.mmd -o /tmp/flow.png -f png
```

## Document Tests (requires Pandoc + Typst)

```bash
# Markdown → PDF (via Pandoc + Typst)
ktw-docs render testcases/sample-doc.md -o /tmp/sample.pdf -t pdf

# Markdown → DOCX
ktw-docs render testcases/sample-doc.md -o /tmp/sample.docx -t docx

# Markdown → HTML
ktw-docs render testcases/sample-doc.md -o /tmp/sample.html -t html

# With Kroki filter (diagrams embedded in document)
ktw-docs render testcases/doc-with-diagrams.md -o /tmp/diagrams.pdf -t pdf \
  --filter "ktw-docs filter"
```

## Server Mode Tests

```bash
# Start server
ktw-docs server start

# Health check
curl http://127.0.0.1:8890/health

# Capabilities
curl http://127.0.0.1:8890/capabilities | jq .

# Render diagram via API
curl -X POST http://127.0.0.1:8890/diagram \
  -H "Content-Type: application/json" \
  -d '{"type":"mermaid","source":"graph TD; A-->B","format":"svg"}' \
  -o /tmp/api-test.svg

# Stop server
ktw-docs server stop
```

## Check Tool Availability

```bash
ktw-docs check
```
