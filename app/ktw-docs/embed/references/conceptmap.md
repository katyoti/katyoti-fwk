# Concept Map

Reference for generating an interactive HTML concept map using Vis.js vis-network from a text or markdown file.

---

## Purpose

Generate a self-contained, interactive HTML concept map from a `.txt` or `.md` file containing explanatory, conceptual, or thematic content. Extract main concepts, subthemes, and logical relationships from unstructured content and visualize them as nodes and directed edges using Vis.js (vis-network).

## Scope

Input: a `.txt` or `.md` file containing explanatory, conceptual, or thematic content.

Output: a single self-contained HTML file named `map_[detected_topic].html`, saved in the same directory as the input file.

## Process

### Step 1 -- Extract concepts

1. Read the input file in full.
2. Identify main themes, subthemes, and key terms.
3. Convert each key concept into a node.

### Step 2 -- Map relationships

Detect logical and hierarchical relations between concepts. Label each edge with a short, descriptive verb:

`causes`, `contributes to`, `depends on`, `evolves into`, `results in`, `influences`, `generates`, `creates`, `culminates in`

### Step 3 -- Build nodes and edges

Node structure:

```json
{"id": "conceito_id", "label": "Conceito", "title": "<b>Concept:</b> Conceito<br><i>Drag to position, double-click to release.</i>"}
```

Edge structure:

```json
{"from": "conceito_origem", "to": "conceito_destino", "label": "verbo", "title": "<b>Relationship:</b> verbo"}
```

### Step 4 -- Apply visual and physics configuration

Use these exact Vis.js options:

```js
// Nodes
shape: "dot",
color: {
    border: "#4285F4",
    background: "#ffffff",
    highlight: { border: "#34A853", background: "#e6f4ea" }
},
font: { size: 14, color: "#3c4043" },
borderWidth: 2,
size: 20

// Edges
color: { color: "#dee2e6", highlight: "#34A853" },
arrows: { to: { enabled: true, scaleFactor: 0.7 } },
font: { align: "middle", size: 12, color: "#5f6368" },
width: 2

// Physics
physics: {
    solver: "forceAtlas2Based",
    forceAtlas2Based: {
        gravitationalConstant: -50,
        centralGravity: 0.005,
        springLength: 100,
        springConstant: 0.18
    },
    maxVelocity: 146,
    minVelocity: 0.1,
    stabilization: { iterations: 150 }
}
```

### Step 5 -- Add interactivity

```js
// Fix node on drag end
network.on("dragEnd", (params) => {
    if (params.nodes.length > 0) {
        nodes.update({ id: params.nodes[0], fixed: true });
    }
});

// Release node on double click
network.on("doubleClick", (params) => {
    if (params.nodes.length > 0) {
        nodes.update({ id: params.nodes[0], fixed: false });
    }
});
```

### Step 6 -- Assemble the HTML file

Produce a single self-contained HTML file with this structure:

```html
<head>
    <title>Concept Map — [DETECTED TOPIC]</title>
    <script src="https://unpkg.com/vis-network/standalone/umd/vis-network.min.js"></script>
    <link href="https://unpkg.com/vis-network/styles/vis-network.min.css" rel="stylesheet" />
</head>
<body>
    <div id="map"></div>
    <script type="text/javascript">
        // nodes, edges, options, and interactive network initialization
    </script>
</body>
```

### Step 7 -- Name and save the output

Auto-name the file based on the detected topic:

```
map_[detected_topic].html
```

Save in the same directory as the input file.

## Constraints

- All relationships must derive from the source text -- no speculative or subjective connections.
- Keep node labels clear and concise.
- Output must be a single self-contained HTML file with valid Vis.js syntax.
- No filler or unrelated content in the map.
