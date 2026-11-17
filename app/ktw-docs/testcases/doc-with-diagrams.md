---
title: "Document with Embedded Diagrams"
author: "ktw-docs Test Suite"
date: 2026-03-08
---

# Architecture Overview

This document tests the Pandoc Kroki filter — diagram code blocks should be replaced with rendered images when using `--filter "ktw-docs filter"`.

## System Flow

```mermaid
graph TD
    User[User] -->|request| API[API Gateway]
    API --> Auth[Auth Service]
    API --> Core[Core Service]
    Core --> DB[(Database)]
    Core --> Cache[(Redis Cache)]
    Auth --> DB
```

## Class Diagram

```plantuml
@startuml
class Server {
  +Start()
  +Stop()
  +handleRender()
  +handleDiagram()
}

class KrokiClient {
  +Render()
  +RenderCached()
}

class PandocExecutor {
  +Render()
  +CheckPandoc()
}

Server --> KrokiClient
Server --> PandocExecutor
@enduml
```

## Network Topology

```graphviz
digraph {
    rankdir=TB
    node [shape=record]

    lb [label="Load Balancer"]
    web1 [label="Web Server 1"]
    web2 [label="Web Server 2"]
    db [label="PostgreSQL"]
    cache [label="Redis"]

    lb -> web1
    lb -> web2
    web1 -> db
    web2 -> db
    web1 -> cache
    web2 -> cache
}
```

## Sequence Diagram

```mermaid
sequenceDiagram
    participant U as User
    participant S as ktw-agents
    participant D as ktw-docs
    participant K as Kroki

    U->>S: Create document
    S->>D: POST /render
    D->>D: Run Pandoc with filter
    D->>K: POST /mermaid/svg
    K-->>D: SVG image
    D-->>S: PDF output path
    S-->>U: Document ready
```

## Entity Relationship

```erd
[Person]
*name
height
weight
+birth_location_id

[Location]
*id
city
state
country

Person *--1 Location
```

## D2 Architecture

```d2
direction: right

client: Client App
server: Backend {
  api: REST API
  worker: Background Worker
  db: PostgreSQL
}

client -> server.api: HTTPS
server.api -> server.db: SQL
server.api -> server.worker: Queue
server.worker -> server.db: SQL
```

## Conclusion

If the Kroki filter is working, all diagram code blocks above should appear as rendered images in the output document.
