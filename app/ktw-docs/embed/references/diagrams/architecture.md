# Architecture Diagrams

Syntax reference for C4 model (via PlantUML) and Structurizr DSL.

---

## C4 with PlantUML

C4 model has 4 levels: Context, Container, Component, Code. Use the C4-PlantUML library.

> Quick syntax only. For the full C4-PlantUML reference (directional
> relationships, layout constraints, the layout-conflict rules) load
> `ktw-docs:/skill/plantuml-syntax` → [plantuml/c4-plantuml.md](plantuml/c4-plantuml.md);
> layout science: [mermaid/c4-layout-science.md](mermaid/c4-layout-science.md).

### System Context Diagram (Level 1)

```c4plantuml
@startuml
!include https://raw.githubusercontent.com/plantuml-stdlib/C4-PlantUML/master/C4_Context.puml

Person(user, "Customer", "A user of the system")
System(sys, "E-Commerce Platform", "Allows customers to browse and buy products")
System_Ext(payment, "Payment Gateway", "Processes payments")
System_Ext(email, "Email Service", "Sends notifications")

Rel(user, sys, "Uses", "HTTPS")
Rel(sys, payment, "Processes payments", "API")
Rel(sys, email, "Sends emails", "SMTP")
@enduml
```

Primitives: `Person(alias, label, desc)`, `System(alias, label, desc)`, `System_Ext(...)`, `System_Boundary(alias, label)`

### Container Diagram (Level 2)

```c4plantuml
@startuml
!include https://raw.githubusercontent.com/plantuml-stdlib/C4-PlantUML/master/C4_Container.puml

Person(user, "Customer")

System_Boundary(sys, "E-Commerce Platform") {
    Container(web, "Web App", "React", "SPA frontend")
    Container(api, "API", "Node.js", "REST API")
    ContainerDb(db, "Database", "PostgreSQL", "Stores products, orders")
    ContainerQueue(mq, "Message Queue", "RabbitMQ", "Async processing")
}

System_Ext(payment, "Stripe", "Payment processing")

Rel(user, web, "Uses", "HTTPS")
Rel(web, api, "Calls", "JSON/HTTPS")
Rel(api, db, "Reads/Writes", "SQL")
Rel(api, mq, "Publishes", "AMQP")
Rel(api, payment, "Charges", "API")
@enduml
```

Primitives: `Container(...)`, `ContainerDb(...)`, `ContainerQueue(...)`, `Container_Ext(...)`

### Component Diagram (Level 3)

```c4plantuml
@startuml
!include https://raw.githubusercontent.com/plantuml-stdlib/C4-PlantUML/master/C4_Component.puml

Container_Boundary(api, "API Service") {
    Component(auth, "Auth Controller", "Express", "Handles authentication")
    Component(orders, "Order Controller", "Express", "Manages orders")
    Component(repo, "Order Repository", "Sequelize", "Data access layer")
    Component(svc, "Payment Service", "Module", "Payment integration")
}

ContainerDb(db, "Database", "PostgreSQL")
System_Ext(stripe, "Stripe")

Rel(auth, orders, "Delegates to")
Rel(orders, repo, "Uses")
Rel(orders, svc, "Calls")
Rel(repo, db, "SQL")
Rel(svc, stripe, "API")
@enduml
```

### Relationship Options

```
Rel(from, to, label)
Rel(from, to, label, technology)
Rel_D(from, to, label)        ' downward
Rel_U(from, to, label)        ' upward
Rel_L(from, to, label)        ' left
Rel_R(from, to, label)        ' right
Rel_Back(from, to, label)     ' reverse
BiRel(a, b, label)            ' bidirectional
```

---

## Editable drawio diagrams

For an editable architecture diagram (ArchiMate, C4, BPMN, AWS, UML) rather than a static image, write a logical diagram and render it with the drawio engine: the `diagram-emit` agent tool or `ktw-docs drawio render`. The shapes and styles come from the viewpoint's templates: `ktw-docs:/reference/drawio-authoring.md`.

## Structurizr DSL

Declarative C4 modeling language. Structurizr is more concise than C4-PlantUML for large models.

### Full Example

```structurizr
workspace {
    model {
        user = person "Customer" "Browses and purchases products"

        enterprise "Corp" {
            webapp = softwareSystem "Web Platform" "Main e-commerce system" {
                spa = container "SPA" "React" "Frontend application"
                api = container "API" "Node.js + Express" "Backend REST API" {
                    authCtrl = component "Auth Controller" "Handles login/signup"
                    orderCtrl = component "Order Controller" "Manages orders"
                    repo = component "Repository" "Data access layer"
                }
                db = container "Database" "PostgreSQL" "Stores all data" {
                    tags "Database"
                }
                cache = container "Cache" "Redis" "Session and query cache"
            }
        }

        payment = softwareSystem "Stripe" "Payment processing" {
            tags "External"
        }

        user -> spa "Uses" "HTTPS"
        spa -> api "Calls" "JSON/HTTPS"
        authCtrl -> repo "Uses"
        orderCtrl -> repo "Uses"
        orderCtrl -> payment "Charges" "API"
        repo -> db "Reads/Writes" "SQL"
        api -> cache "Caches" "Redis protocol"
    }

    views {
        systemContext webapp "Context" {
            include *
            autolayout lr
        }
        container webapp "Containers" {
            include *
            autolayout lr
        }
        component api "Components" {
            include *
            autolayout lr
        }
    }
}
```

### Key Primitives

```
person <name> <description>
softwareSystem <name> <description> { ... }
container <name> <technology> <description> { ... }
component <name> <description>
-> <destination> <description> <technology>     # relationship
```

### Views

```
systemLandscape <key> { include * }
systemContext <system> <key> { include * }
container <system> <key> { include * }
component <container> <key> { include * }
autolayout lr|tb|rl|bt
```
