# Flowcharts & Graph Diagrams

Syntax reference for Mermaid, GraphViz (DOT), and D2.

---

## Mermaid

> Quick syntax only. For the full per-type Mermaid references (23 diagram
> types, theming, parser gotchas), load `ktw-docs:/skill/mermaid-syntax`,
> which routes into [mermaid/](mermaid/).

### Flowchart

```mermaid
graph TD
    A[Start] --> B{Decision?}
    B -->|Yes| C[Action 1]
    B -->|No| D[Action 2]
    C --> E[End]
    D --> E
```

Node shapes: `[rectangle]`, `(rounded)`, `{diamond}`, `([stadium])`, `[[subroutine]]`, `[(cylinder)]`, `((circle))`, `>asymmetric]`, `{hexagon}`, `[/parallelogram/]`, `[\trapezoid\]`

Directions: `TD` (top-down), `LR` (left-right), `BT`, `RL`

Links: `-->` arrow, `---` line, `-.->` dotted, `==>` thick, `--text-->` labeled

Subgraphs:
```mermaid
graph TD
    subgraph Group1[Label]
        A --> B
    end
    subgraph Group2
        C --> D
    end
    B --> C
```

### Sequence Diagram

```mermaid
sequenceDiagram
    participant A as Alice
    participant B as Bob
    A->>B: Hello
    B-->>A: Hi back
    A->>B: Request
    activate B
    B-->>A: Response
    deactivate B
    Note over A,B: Shared note
    alt Success
        B->>A: OK
    else Failure
        B->>A: Error
    end
    loop Every minute
        A->>B: Ping
    end
```

Arrows: `->>` solid, `-->>` dashed, `-x` cross, `-)` open

### Gantt Chart

```mermaid
gantt
    title Project Plan
    dateFormat YYYY-MM-DD
    section Phase 1
        Task A :a1, 2025-01-01, 30d
        Task B :after a1, 20d
    section Phase 2
        Task C :2025-02-15, 25d
        Milestone :milestone, m1, 2025-03-15, 0d
```

### Pie Chart

```mermaid
pie title Distribution
    "Category A" : 45
    "Category B" : 30
    "Category C" : 25
```

### State Diagram

```mermaid
stateDiagram-v2
    [*] --> Idle
    Idle --> Processing : submit
    Processing --> Done : success
    Processing --> Error : failure
    Error --> Idle : retry
    Done --> [*]
    state Processing {
        [*] --> Validating
        Validating --> Executing
    }
```

### Git Graph

```mermaid
gitgraph
    commit
    branch develop
    checkout develop
    commit
    commit
    checkout main
    merge develop
    commit
```

### Class Diagram

```mermaid
classDiagram
    class Animal {
        +String name
        +int age
        +eat() void
    }
    class Dog {
        +fetch() void
    }
    Animal <|-- Dog
    Animal "1" --> "*" Food
```

### Entity Relationship

```mermaid
erDiagram
    CUSTOMER ||--o{ ORDER : places
    ORDER ||--|{ LINE-ITEM : contains
    PRODUCT ||--o{ LINE-ITEM : "is in"
    CUSTOMER {
        string name
        string email
    }
    ORDER {
        int id
        date created
    }
```

### Mindmap

```mermaid
mindmap
    root((Topic))
        Branch A
            Leaf 1
            Leaf 2
        Branch B
            Leaf 3
        Branch C
```

### Timeline

```mermaid
timeline
    title History
    2020 : Event A
    2021 : Event B : Event C
    2022 : Event D
```

---

## GraphViz (DOT)

### Directed Graph

```graphviz
digraph G {
    rankdir=LR;
    node [shape=box, style=filled, fillcolor="#e8f0fe"];

    A [label="Start"];
    B [label="Process", shape=ellipse];
    C [label="Decision", shape=diamond];
    D [label="End"];

    A -> B;
    B -> C;
    C -> D [label="yes"];
    C -> B [label="no", style=dashed];
}
```

### Undirected Graph

```graphviz
graph G {
    layout=neato;
    A -- B -- C;
    B -- D;
    C -- D;
}
```

### Cluster Subgraphs

```graphviz
digraph G {
    subgraph cluster_0 {
        label="Group A";
        style=filled; color=lightgrey;
        a0 -> a1 -> a2;
    }
    subgraph cluster_1 {
        label="Group B";
        color=blue;
        b0 -> b1;
    }
    a1 -> b0;
}
```

Key shapes: `box`, `ellipse`, `diamond`, `circle`, `record`, `plaintext`, `doublecircle`, `Mrecord`

Layouts: `dot` (hierarchical), `neato` (spring), `fdp` (force-directed), `circo` (circular), `twopi` (radial)

---

## D2

### Basic

```d2
server: Web Server
db: Database {
    shape: cylinder
}
server -> db: queries
server -> client: responses
client: Browser {
    shape: rectangle
}
client -> server: requests
```

### Styled

```d2
direction: right

service: API Service {
    style: {
        fill: "#e8f0fe"
        stroke: "#4285f4"
        border-radius: 8
    }
}

database: PostgreSQL {
    shape: cylinder
    style.fill: "#fce8e6"
}

cache: Redis {
    shape: hexagon
}

service -> database: read/write
service -> cache: get/set
```

### Containers (Nested)

```d2
cloud: AWS {
    vpc: VPC {
        web: Web Tier {
            lb: Load Balancer
            app1: App Server 1
            app2: App Server 2
            lb -> app1
            lb -> app2
        }
        data: Data Tier {
            db: RDS
        }
    }
    web.app1 -> data.db
    web.app2 -> data.db
}
```

### Sequence Diagram

```d2
shape: sequence_diagram
alice -> bob: Hello
bob -> charlie: Forward
charlie -> bob: Reply
bob -> alice: Response
```

Shapes: `rectangle`, `oval`, `cylinder`, `diamond`, `hexagon`, `circle`, `queue`, `package`, `class`, `sequence_diagram`
