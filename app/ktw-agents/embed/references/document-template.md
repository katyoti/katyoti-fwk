# Requirements Analysis Document Template

Use this template to structure the output document. Interview the user dimension
by dimension, filling each section. Mark sections as "TBD" when the user doesn't
have the information yet.

---

# Requirements Analysis — {Project Name}

**Date**: {date}
**Author**: {user} (assisted by requirements-analysis agent)
**Status**: Draft | Review | Validated

## 1. Context & Scope

### 1.1 Executive Summary
> {One sentence: what exists today and what is needed}

### 1.2 Scope

| In Scope | Out of Scope |
|----------|-------------|
| {item} | {item} |

### 1.3 Dependencies
- {Project/system/evolution this depends on or impacts}

### 1.4 Hypotheses & Decisions

| # | Hypothesis / Decision | Status | Date |
|---|----------------------|--------|------|
| H1 | {assumption} | To validate | {date} |
| D1 | {decision taken} | Decided | {date} |

---

## 2. Functional Requirements

### 2.1 Use Cases

| # | Use Case | Actor | Precondition | Main Flow | Postcondition | Priority |
|---|----------|-------|-------------|-----------|---------------|----------|
| UC1 | {name} | {actor} | {precondition} | {steps} | {result} | Must/Should/Could |

### 2.2 User Profiles

| Profile | Role | Permissions | Frequency | Key Expectations |
|---------|------|------------|-----------|-----------------|
| {name} | {role} | {what they can do} | {daily/weekly/monthly} | {what matters to them} |

### 2.3 Business Process

{Describe the business process flow. Which steps? Which actors? Which applications at each step?}

```
Step 1: [Actor] → [Action] → [System]
Step 2: [System] → [Processing] → [Result]
...
```

### 2.4 Functional Coverage

| Business Capability | Current Coverage | Target Coverage | Gap |
|--------------------|-----------------|----------------|-----|
| {capability} | {partial/full/none} | {full} | {what's missing} |

---

## 3. Non-Functional Requirements (DICP)

### 3.1 Disponibilité (Availability)

| Attribute | Requirement |
|-----------|-------------|
| Service hours | {e.g., 24/7, business hours 8h-20h} |
| Uptime SLA | {e.g., 99.9%} |
| Maximum acceptable downtime | {e.g., 4h/month} |
| Planned maintenance windows | {e.g., Sunday 02:00-06:00} |

### 3.2 Intégrité (Integrity)

| Attribute | Requirement |
|-----------|-------------|
| Data validation rules | {constraints, checksums} |
| Consistency model | {eventual, strong, transactional} |
| Maximum acceptable data loss (RPO) | {e.g., 0, 1h, 24h} |
| Data quality expectations | {deduplication, cleansing, enrichment} |

### 3.3 Confidentialité (Confidentiality)

| Attribute | Requirement |
|-----------|-------------|
| Data classification | {public, internal, confidential, restricted} |
| Access control model | {RBAC, ABAC, ACL} |
| Encryption | {at rest, in transit, end-to-end} |
| Anonymization needs | {PII handling, GDPR} |

### 3.4 Preuve (Traceability)

| Attribute | Requirement |
|-----------|-------------|
| Audit trail | {what actions to log} |
| Retention period | {e.g., 5 years} |
| Regulatory compliance | {GDPR, SOX, PCI-DSS, etc.} |
| Non-repudiation | {digital signatures, timestamps} |

### 3.5 Continuity & Recovery

| Attribute | Requirement |
|-----------|-------------|
| Recovery Time Objective (RTO) | {max time to restore service} |
| Recovery Point Objective (RPO) | {max acceptable data loss} |
| Backup strategy | {frequency, location, testing} |
| Disaster recovery plan | {active-active, active-passive, cold site} |

### 3.6 Quality of Service

| Attribute | Requirement |
|-----------|-------------|
| Response time (P95) | {e.g., < 500ms} |
| Throughput | {e.g., 1000 req/s} |
| Monitoring | {health checks, dashboards, alerts} |
| SLA penalties | {if applicable} |

---

## 4. Data Vision

### 4.1 Data Objects

| Object | Description | Owner | Source | Sensitivity |
|--------|-------------|-------|--------|-------------|
| {entity} | {what it represents} | {team/system} | {where it comes from} | {public/internal/confidential} |

### 4.2 Data Model

{Conceptual data model — entities and relationships}

```
[Customer] 1───* [Order] *───* [Product]
[Order] 1───1 [Payment]
```

### 4.3 Data Lifecycle

| Phase | Rules |
|-------|-------|
| Creation | {who creates, validation, defaults} |
| Update | {who can update, versioning, conflict resolution} |
| Archival | {when, where, format} |
| Deletion | {retention policy, right to be forgotten} |

---

## 5. Flow Vision

### 5.1 Inter-Application Flow Matrix

| ID | Name | Description | Emitter | Receiver | Protocol | Mode | Frequency | Data Object |
|----|------|-------------|---------|----------|----------|------|-----------|-------------|
| F1 | {name} | {what flows} | {app} | {app} | {REST/MQ/SFTP} | {sync/async/batch} | {real-time/daily} | {entity} |

### 5.2 Triggers

| Flow | Trigger | Condition |
|------|---------|-----------|
| F1 | {event/schedule/manual} | {when it fires} |

### 5.3 Transformations

| Flow | Source Format | Target Format | Transformation Rules |
|------|-------------|--------------|---------------------|
| F1 | {JSON/CSV/XML} | {JSON/DB} | {mapping, enrichment, filtering} |

---

## 6. Application Landscape

### 6.1 Applications Involved

| Application | Role | Current | Target | Interface Type |
|-------------|------|---------|--------|---------------|
| {app} | {what it does} | {yes/no} | {yes/no} | {API/DB/File/MQ} |

### 6.2 Technology Stack

| Layer | Technology | Notes |
|-------|-----------|-------|
| Frontend | {React/Angular/mobile} | |
| Backend | {Go/Java/Python} | |
| Database | {PostgreSQL/MongoDB} | |
| Messaging | {Kafka/RabbitMQ} | |
| Cloud | {AWS/Azure/GCP/on-prem} | |

---

## 7. Journey Map (Plateaux)

### 7.1 Current State (Plateau 0)
{Description of the current situation — what exists, what works, what doesn't}

### 7.2 Target State (Plateau N)
{Description of the desired end state}

### 7.3 Intermediate Plateaux

| Plateau | Description | Key Deliverables | Dependencies |
|---------|-------------|-----------------|-------------|
| P1 | {first stable milestone} | {what's delivered} | {what it needs} |
| P2 | {second stable milestone} | {what's delivered} | {what it needs} |

### 7.4 Transition Strategy
{How to move from current to target. Migration approach, data migration, cutover strategy.}

### 7.5 Deployment Strategy
{Big bang / phased / canary / blue-green. Rollback plan.}

---

## 8. Volumetry

| Metric | Current | Target (Year 1) | Target (Year 3) |
|--------|---------|-----------------|-----------------|
| Records | {count} | {count} | {count} |
| Storage | {GB/TB} | {GB/TB} | {GB/TB} |
| Transactions/day | {count} | {count} | {count} |
| Peak load (req/s) | {count} | {count} | {count} |
| Concurrent users | {count} | {count} | {count} |
| Batch volume | {records/batch} | | |
| Growth rate | {%/year} | | |

---

## 9. Exception Handling

| # | Scenario | Detection | Handling | Escalation | Recovery |
|---|----------|-----------|----------|-----------|----------|
| E1 | {what goes wrong} | {how detected} | {retry/fallback/compensate} | {who is notified} | {how to return to normal} |

### Degraded Modes

| Component Failure | Impact | Degraded Behavior | Acceptable Duration |
|-------------------|--------|-------------------|-------------------|
| {component} | {what breaks} | {how system continues} | {max time in degraded mode} |

---

## 10. Risks & Open Questions

### Identified Risks

| # | Risk | Probability | Impact | Mitigation |
|---|------|-------------|--------|-----------|
| R1 | {risk description} | High/Medium/Low | High/Medium/Low | {how to mitigate} |

### Open Questions

| # | Question | Owner | Due Date | Status |
|---|----------|-------|----------|--------|
| Q1 | {question to resolve} | {who} | {when} | Open/Resolved |

### Assumptions to Validate

| # | Assumption | How to Validate | Status |
|---|-----------|----------------|--------|
| A1 | {assumption} | {validation method} | Pending/Validated/Invalidated |
