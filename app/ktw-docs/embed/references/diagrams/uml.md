# UML Diagrams

Syntax reference for PlantUML and Nomnoml.

---

## PlantUML

> Quick syntax only. For the full per-type PlantUML references (sequence,
> class, activity, state, ER, component, deployment, use case + styling and
> common errors), load `ktw-docs:/skill/plantuml-syntax`, which routes into
> [plantuml/](plantuml/).

### Class Diagram

```plantuml
@startuml
class User {
    -id: Long
    -name: String
    -email: String
    +getName(): String
    +setEmail(email: String): void
}

class Order {
    -id: Long
    -total: BigDecimal
    +addItem(item: Item): void
}

class Item {
    -name: String
    -price: BigDecimal
}

User "1" --> "*" Order : places
Order "*" --> "*" Item : contains
@enduml
```

Relationships: `--|>` inheritance, `--*` composition, `--o` aggregation, `-->` association, `..>` dependency, `..|>` implementation

Visibility: `-` private, `+` public, `#` protected, `~` package

### Sequence Diagram

```plantuml
@startuml
actor User
participant "Web App" as web
participant "API" as api
database "DB" as db

User -> web: Login request
activate web
web -> api: POST /auth
activate api
api -> db: SELECT user
activate db
db --> api: user record
deactivate db
alt valid credentials
    api --> web: 200 + token
    web --> User: Dashboard
else invalid
    api --> web: 401
    web --> User: Error message
end
deactivate api
deactivate web
@enduml
```

### Activity Diagram

```plantuml
@startuml
start
:Receive request;
if (Authenticated?) then (yes)
    if (Authorized?) then (yes)
        :Process request;
        :Return response;
    else (no)
        :Return 403;
    endif
else (no)
    :Return 401;
endif
stop
@enduml
```

### State Diagram

```plantuml
@startuml
[*] --> Draft
Draft --> Review : submit
Review --> Approved : approve
Review --> Draft : reject
Approved --> Published : publish
Published --> Archived : archive
Archived --> [*]

state Review {
    [*] --> PeerReview
    PeerReview --> ManagerReview
    ManagerReview --> [*]
}
@enduml
```

### Component Diagram

```plantuml
@startuml
package "Frontend" {
    [Web App] as web
    [Mobile App] as mobile
}

package "Backend" {
    [API Gateway] as gw
    [Auth Service] as auth
    [Order Service] as order
}

database "PostgreSQL" as db
queue "RabbitMQ" as mq

web --> gw
mobile --> gw
gw --> auth
gw --> order
order --> db
order --> mq
@enduml
```

### Use Case Diagram

```plantuml
@startuml
left to right direction
actor Customer
actor Admin

rectangle "E-Commerce" {
    Customer --> (Browse Products)
    Customer --> (Place Order)
    Customer --> (Track Order)
    Admin --> (Manage Products)
    Admin --> (Process Orders)
    (Place Order) .> (Payment) : <<include>>
    (Track Order) .> (Send Notification) : <<extend>>
}
@enduml
```

### Deployment Diagram

```plantuml
@startuml
node "Web Server" {
    artifact "app.war"
}
node "App Server" {
    artifact "api.jar"
}
database "MySQL" {
    artifact "schema.sql"
}
cloud "CDN" {
    artifact "static assets"
}
[Web Server] --> [App Server] : REST
[App Server] --> [MySQL] : JDBC
[Web Server] --> [CDN] : assets
@enduml
```

### Timing Diagram

```plantuml
@startuml
robust "Server" as s
concise "Client" as c

@0
s is Idle
c is Disconnected

@100
c is Connecting
s is Accepting

@200
c is Connected
s is Active

@500
c is Disconnected
s is Idle
@enduml
```

---

## Nomnoml

Quick, lightweight UML class diagrams.

### Class Diagram

```nomnoml
[User|
    -id: Long;
    -name: String
    |
    +getName(): String;
    +setName(n: String): void
]

[Order|
    -id: Long;
    -items: List
]

[User] 1 -> * [Order]
[Order] *-> * [Product]
```

### Relationships

```nomnoml
[A] -> [B]
[A] --> [B]
[A] -:> [B]
[A] --:> [B]
[A] -o [B]
[A] -+ [B]
[A] o- [B]
```

Arrows: `->` association, `-->` dependency, `-:>` inheritance, `--:>` implementation, `-o` aggregation, `-+` composition, `o-` note

### Styled

```nomnoml
#direction: right
#.important: fill=#f44 visual=rhomb
[<important> Critical Service]
[<abstract> BaseHandler]
[<instance> ConcreteHandler]
[BaseHandler] <:- [ConcreteHandler]
[ConcreteHandler] -> [Critical Service]
```

Classifiers: `<abstract>`, `<instance>`, `<note>`, `<reference>`, `<package>`, `<frame>`, `<database>`, `<start>`, `<end>`, `<state>`, `<choice>`, `<sender>`, `<receiver>`, `<actor>`, `<usecase>`
