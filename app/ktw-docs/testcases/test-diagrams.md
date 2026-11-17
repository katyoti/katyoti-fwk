# Test des Diagrammes

Cette page teste les differents types de diagrammes supportes.

## 1. Draw.io (lien image)

Diagramme draw.io integre via un lien image :

![Flux simple entre deux applications](figures/test_simple_diagram.drawio)

## 2. Mermaid

### 2.1 Code block Mermaid

<!-- yaml:
name: diagram_001
-->
```mermaid
graph TD
    A[Client Web] -->|Requete HTTP| B[API Gateway]
    B --> C[Service Authentification]
    B --> D[Service Metier]
    D --> E[(Base de donnees)]
    C --> F[LDAP / Active Directory]
    D --> G[File MQ]
    G --> H[Service Batch]
```

### 2.2 Fichier Mermaid (lien)

*(Non applicable - pas de fichier .mmd dans ce workspace)*

## 3. PlantUML

### 3.1 Code block PlantUML

<!-- yaml:
name: diagram_002
-->
```plantuml
@startuml
actor Utilisateur as user
participant "Application Source" as src
participant "Connecteur" as conn
participant "Application Cible" as dest
database "Base de donnees" as db

user -> src : Declenche le flux
src -> conn : Envoie les donnees
conn -> conn : Transformation\nformat pivot
conn -> dest : Transmet les donnees
dest -> db : Persiste les donnees
dest --> conn : Accusé de reception
conn --> src : Confirmation
src --> user : Resultat
@enduml
```

## 4. Diagramme C4 (PlantUML C4)

### 4.1 C4 Context Diagram

<!-- yaml:
name: diagram_003
-->
```plantuml
@startuml
!include <C4/C4_Context>

title System Context Diagram - Plateforme d'Integration

Person(user, "Utilisateur Metier", "Gere les flux d'integration")
Person(admin, "Administrateur", "Configure et supervise la plateforme")

System(platform, "Plateforme d'Integration", "Orchestre les echanges de donnees entre applications")

System_Ext(erp, "ERP", "Systeme de gestion integre")
System_Ext(crm, "CRM", "Gestion de la relation client")
System_Ext(bi, "BI / Reporting", "Analyse et tableaux de bord")
System_Ext(partenaire, "SI Partenaire", "Systeme d'information externe")

Rel(user, platform, "Configure les flux", "IHM Web")
Rel(admin, platform, "Supervise", "Console d'administration")
Rel(erp, platform, "Envoie / recoit des donnees", "API REST / MQ")
Rel(crm, platform, "Envoie / recoit des donnees", "API REST")
Rel(platform, bi, "Transmet les indicateurs", "API REST / JDBC")
Rel(platform, partenaire, "Echange de fichiers", "SFTP / AS2")

@enduml
```

### 4.2 C4 Container Diagram

<!-- yaml:
name: diagram_004
-->
```plantuml
@startuml
!include <C4/C4_Container>

title Container Diagram - Plateforme d'Integration

Person(user, "Utilisateur Metier")

System_Boundary(platform, "Plateforme d'Integration") {
    Container(webapp, "Application Web", "React", "Interface de configuration et supervision")
    Container(api, "API Backend", "Go", "Services REST pour la gestion des flux")
    Container(engine, "Moteur d'Integration", "Go", "Orchestration et transformation des donnees")
    Container(scheduler, "Planificateur", "Go", "Declenchement des flux planifies")
    ContainerDb(db, "Base de donnees", "PostgreSQL", "Configuration des flux et metadonnees")
    ContainerQueue(mq, "Bus de Messages", "RabbitMQ", "Communication asynchrone")
    Container(monitoring, "Monitoring", "Prometheus + Grafana", "Supervision et alerting")
}

System_Ext(erp, "ERP")
System_Ext(crm, "CRM")

Rel(user, webapp, "Utilise", "HTTPS")
Rel(webapp, api, "Appelle", "REST API")
Rel(api, db, "Lit/Ecrit", "SQL")
Rel(api, engine, "Declenche", "gRPC")
Rel(scheduler, engine, "Planifie", "gRPC")
Rel(engine, mq, "Publie/Consomme", "AMQP")
Rel(engine, erp, "Echange", "REST/MQ")
Rel(engine, crm, "Echange", "REST")
Rel(engine, db, "Journalise", "SQL")
Rel(monitoring, engine, "Collecte metriques", "HTTP")

@enduml
```

## 5. Diagramme de Sequence Mermaid

<!-- yaml:
name: diagram_005
-->
```mermaid
sequenceDiagram
    participant U as Utilisateur
    participant W as Web App
    participant A as API Gateway
    participant S as Service Metier
    participant D as Base de donnees

    U->>W: Action utilisateur
    W->>A: POST /api/flux
    A->>A: Validation JWT
    A->>S: Traitement metier
    S->>D: INSERT donnees
    D-->>S: OK
    S-->>A: 201 Created
    A-->>W: Response JSON
    W-->>U: Confirmation visuelle
```
