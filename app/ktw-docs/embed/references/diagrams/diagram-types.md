# Diagram Types Reference

Quick reference for selecting the right diagram type and writing its source code.

## Table of Contents

- [Selection Guide](#selection-guide)
- [Flowcharts and Graphs](#flowcharts-and-graphs)
- [UML Diagrams](#uml-diagrams)
- [Architecture Diagrams](#architecture-diagrams)
- [Data Visualization](#data-visualization)
- [Network and Infrastructure](#network-and-infrastructure)
- [Other Diagram Types](#other-diagram-types)

## Selection Guide

| Need | Recommended Type | Alternative |
|---|---|---|
| Flowchart / process flow | `mermaid` | `graphviz`, `d2` |
| Sequence diagram | `mermaid` | `plantuml`, `seqdiag` |
| Class diagram | `plantuml` | `mermaid`, `nomnoml` |
| State machine | `mermaid` | `plantuml` |
| Activity / workflow | `mermaid` | `plantuml`, `actdiag`, `bpmn` |
| Architecture overview | `c4plantuml` | `structurizr`, `d2` |
| ER diagram | `erd` | `dbml`, `mermaid` |
| Database schema | `dbml` | `erd`, `plantuml` |
| Network topology | `nwdiag` | `d2`, `graphviz` |
| Rack layout | `rackdiag` | - |
| Bar/line/scatter chart | `vegalite` | `vega` |
| Pie chart | `mermaid` | `vegalite` |
| Gantt chart | `mermaid` | `plantuml` |
| Business process (BPMN) | `bpmn` | `mermaid` |
| Hand-drawn style | `excalidraw` | - |
| ASCII art to diagram | `ditaa` | `svgbob` |
| Timing / waveform | `wavedrom` | - |
| Byte / protocol fields | `bytefield` | `packetdiag` |
| Wiring harness | `wireviz` | - |
| Git graph | `mermaid` | - |
| Mind map | `plantuml` | `mermaid` |
| Modern styled diagram | `d2` | `mermaid` |

## Flowcharts and Graphs

### Mermaid

Most versatile. Supports flowcharts, sequence, class, state, ER, Gantt, pie, git graph, mind map.

```
graph TD
    A[Start] --> B{Decision}
    B -->|Yes| C[Action 1]
    B -->|No| D[Action 2]
    C --> E[End]
    D --> E
```

Direction: `TD` (top-down), `LR` (left-right), `BT`, `RL`.
Shapes: `[rect]`, `(round)`, `{diamond}`, `([stadium])`, `[[subroutine]]`, `[(cylinder)]`, `((circle))`.

### GraphViz (DOT)

Best for complex directed/undirected graphs with automatic layout.

```
digraph G {
    rankdir=LR;
    node [shape=box];
    A -> B -> C;
    A -> D -> C;
}
```

### D2

Modern declarative syntax with styling support.

```
direction: right
server: Web Server {
  shape: rectangle
}
db: Database {
  shape: cylinder
}
server -> db: queries
```

## UML Diagrams

### PlantUML

Full UML suite. Prefix with `@startuml` / `@enduml`.

**Sequence:**
```
@startuml
Alice -> Bob: Request
Bob --> Alice: Response
@enduml
```

**Class:**
```
@startuml
class User {
  +name: String
  +login(): void
}
class Admin extends User {
  +role: String
}
@enduml
```

**Activity:**
```
@startuml
start
:Step 1;
if (condition?) then (yes)
  :Action A;
else (no)
  :Action B;
endif
stop
@enduml
```

### Nomnoml

Quick, concise class diagrams.

```
[User|name: string;email: string|login();logout()]
[Admin|role: string]
[User] <:- [Admin]
```

## Architecture Diagrams

### C4 PlantUML

C4 model: Context, Container, Component, Code views.

```
@startuml
!include https://raw.githubusercontent.com/plantuml-stdlib/C4-PlantUML/master/C4_Container.puml

Person(user, "User")
System_Boundary(sys, "System") {
    Container(web, "Web App", "React")
    Container(api, "API", "Node.js")
    ContainerDb(db, "Database", "PostgreSQL")
}
Rel(user, web, "Uses")
Rel(web, api, "Calls")
Rel(api, db, "Reads/Writes")
@enduml
```

### Structurizr

C4 model with Structurizr DSL.

```
workspace {
    model {
        user = person "User"
        system = softwareSystem "System" {
            webapp = container "Web App"
            api = container "API"
        }
        user -> webapp "Uses"
        webapp -> api "Calls"
    }
    views {
        container system {
            include *
            autolayout lr
        }
    }
}
```

## Data Visualization

### Vega-Lite

Concise statistical charts (JSON format).

```json
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "data": {
    "values": [
      {"category": "A", "value": 28},
      {"category": "B", "value": 55},
      {"category": "C", "value": 43}
    ]
  },
  "mark": "bar",
  "encoding": {
    "x": {"field": "category", "type": "nominal"},
    "y": {"field": "value", "type": "quantitative"}
  }
}
```

Chart types: `bar`, `line`, `point`, `area`, `rect`, `arc` (pie), `boxplot`.

## Network and Infrastructure

### NwDiag

Network topology.

```
nwdiag {
  network dmz {
    address = "210.x.x.x/24"
    web01 [address = "210.x.x.1"];
    web02 [address = "210.x.x.2"];
  }
  network internal {
    address = "172.x.x.x/24"
    web01 [address = "172.x.x.1"];
    db01;
  }
}
```

### BlockDiag

Simple block diagrams.

```
blockdiag {
  A -> B -> C -> D;
  A -> E -> F -> D;
}
```

## Other Diagram Types

### BPMN

Business process (XML format). Use BPMN 2.0 XML.

### Excalidraw

Hand-drawn style (JSON format). Use Excalidraw JSON schema.

### ERD

Entity-relationship diagrams.

```
[Person]
*name
height
weight

[Pet]
*name
+species

Person *--* Pet
```

### DBML

Database schema.

```
Table users {
  id integer [pk, increment]
  username varchar
  email varchar
}
Table posts {
  id integer [pk, increment]
  user_id integer [ref: > users.id]
  title varchar
}
```

### Ditaa

ASCII art to clean diagrams.

```
+--------+   +-------+
|        |-->| cBLU  |
| Input  |   | Logic |
|        |   |       |
+--------+   +---+---+
                  |
                  v
              +---+---+
              |       |
              |Output |
              |       |
              +-------+
```

### WaveDrom

Digital timing diagrams (JSON).

```json
{ "signal": [
  { "name": "clk",  "wave": "p......" },
  { "name": "data", "wave": "x.345x.", "data": ["a","b","c"] },
  { "name": "req",  "wave": "01..0.." },
  { "name": "ack",  "wave": "0..1..0" }
]}
```
