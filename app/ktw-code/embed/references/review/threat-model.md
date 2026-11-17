# Threat Model

## Purpose

Produce a structured threat model from static code analysis — not a vulnerability scanner, but an attacker's map. Identifies what could be attacked, how, and at what risk. Complements `security.md` (which finds what is broken) by mapping the full attack surface, generating realistic multi-step attack paths, and producing a prioritized test plan.

## Scope

Reads: source code, configuration files, dependency manifests, routing/API definitions, authentication logic, database schemas or ORM models if present. No live tools, no network calls — static analysis only.
Output: structured Markdown threat model report.

## Process

### Phase 1 — Application understanding

Read the codebase to build a working model of the application:

1. **Technology stack**: languages, frameworks, databases, external services, auth libraries
2. **User roles**: enumerate distinct roles and their access levels (anonymous, authenticated, admin, service-to-service, etc.)
3. **Attack surface inventory**: all input boundaries — HTTP endpoints, form fields, file uploads, websocket handlers, CLI arguments, environment variables, message queue consumers, scheduled jobs, webhook receivers
4. **Sensitive assets**: user data, credentials, payment data, PII, secrets, admin functions, privileged operations
5. **Trust boundaries**: where data crosses from untrusted to trusted context (user input → business logic, external API response → internal processing, etc.)

Produce a compact summary table:

| Aspect | Finding |
|--------|---------|
| Stack | [languages, frameworks] |
| Auth mechanism | [JWT / session / OAuth / API key / none] |
| User roles | [list] |
| External integrations | [list] |
| Sensitive assets | [list] |
| Input entry points | [count and types] |

### Phase 2 — Threat identification

For each applicable category below, determine whether it applies based on Phase 1 findings, then enumerate specific threats. Skip categories that genuinely do not apply (e.g., no file upload = skip file upload threats). State explicitly when skipping and why.

#### Category A — Authentication (apply when any auth exists)

| Threat | OWASP | CWE | Default Risk |
|--------|-------|-----|-------------|
| Credential stuffing / brute force | A07 | CWE-307 | High |
| Session fixation / hijacking | A07 | CWE-384 | High |
| JWT manipulation (alg:none, weak secret, claim injection) | A07 | CWE-287 | Critical |
| Password reset flow abuse | A07 | CWE-640 | High |
| Token leakage in logs, URLs, or error responses | A07 | CWE-532 | Medium |
| Insufficient session expiry | A07 | CWE-613 | Medium |
| OAuth misconfigurations (open redirect, state bypass) | A07 | CWE-601 | High |

#### Category B — Access control (apply when multiple roles or resource ownership exists)

| Threat | OWASP | CWE | Default Risk |
|--------|-------|-----|-------------|
| Horizontal privilege escalation (IDOR) | A01 | CWE-639 | High |
| Vertical privilege escalation | A01 | CWE-269 | Critical |
| Missing function-level access control | A01 | CWE-285 | High |
| Forced browsing to unauthenticated endpoints | A01 | CWE-425 | Medium |
| Mass assignment / parameter pollution | A08 | CWE-915 | High |

#### Category C — Injection (apply to all projects with external input)

| Threat | OWASP | CWE | Default Risk |
|--------|-------|-----|-------------|
| SQL injection | A03 | CWE-89 | Critical |
| NoSQL injection | A03 | CWE-943 | Critical |
| Command injection | A03 | CWE-78 | Critical |
| XSS (reflected, stored, DOM) | A03 | CWE-79 | Medium–High |
| Server-side template injection | A03 | CWE-94 | Critical |
| Path traversal | A01 | CWE-22 | High |
| SSRF | A10 | CWE-918 | High |
| XXE | A05 | CWE-611 | High |
| Header injection | A03 | CWE-113 | Medium |

#### Category D — Data exposure (apply when sensitive data is stored or transmitted)

| Threat | OWASP | CWE | Default Risk |
|--------|-------|-----|-------------|
| Sensitive data in transit without encryption | A02 | CWE-319 | High |
| PII or secrets in logs | A02 | CWE-532 | Medium |
| Sensitive data in error messages | A05 | CWE-200 | Medium |
| Insecure direct object references exposing data | A01 | CWE-639 | High |
| Caching of sensitive responses | A02 | CWE-524 | Medium |

#### Category E — Cryptographic failures (apply when crypto is used)

| Threat | OWASP | CWE | Default Risk |
|--------|-------|-----|-------------|
| Weak hashing for passwords (MD5, SHA1, unsalted) | A02 | CWE-916 | Critical |
| Hard-coded or weak encryption keys | A02 | CWE-321 | Critical |
| Predictable random values used for security tokens | A02 | CWE-338 | High |
| Insecure cipher modes (ECB, no IV) | A02 | CWE-326 | High |

#### Category F — File upload (apply when file upload functionality exists)

| Threat | OWASP | CWE | Default Risk |
|--------|-------|-----|-------------|
| Unrestricted file type upload (webshell) | A04 | CWE-434 | Critical |
| Path traversal via filename | A01 | CWE-22 | High |
| Zip bomb / decompression attack | A04 | CWE-400 | Medium |
| MIME type spoofing | A04 | CWE-434 | High |

#### Category G — API security (apply when REST/GraphQL/gRPC APIs exist)

| Threat | OWASP | CWE | Default Risk |
|--------|-------|-----|-------------|
| Broken object level authorization | A01 | CWE-639 | High |
| Excessive data exposure in responses | A01 | CWE-213 | Medium |
| Lack of rate limiting | A04 | CWE-770 | Medium |
| GraphQL introspection enabled in production | A05 | CWE-200 | Low |
| GraphQL batching / DoS | A04 | CWE-400 | Medium |
| API versioning exposing deprecated vulnerable endpoints | A05 | CWE-1059 | Medium |

#### Category H — Business logic (apply to all projects)

| Threat | OWASP | CWE | Default Risk |
|--------|-------|-----|-------------|
| Workflow bypass (skipping required steps) | A04 | CWE-840 | High |
| Race condition exploitation | A04 | CWE-362 | High |
| Negative value / overflow abuse | A04 | CWE-190 | High |
| Replay attacks on non-idempotent operations | A04 | CWE-294 | Medium |

#### Category I — Infrastructure & configuration (apply to all projects)

| Threat | OWASP | CWE | Default Risk |
|--------|-------|-----|-------------|
| Debug mode / stack traces in production | A05 | CWE-209 | Medium |
| Default or weak admin credentials | A07 | CWE-521 | Critical |
| Missing security headers (CSP, HSTS, X-Frame-Options) | A05 | CWE-693 | Medium |
| Dependency with known CVE | A06 | CWE-1104 | Variable |
| Secrets committed to repository | A02 | CWE-798 | Critical |

### Phase 3 — Risk scoring

For each threat identified, calculate a risk score:

- **Impact**: Low (1) / Medium (2) / High (3) / Critical (4)
- **Likelihood**: Low (1) / Medium (2) / High (3)
- **Risk score**: Impact × Likelihood → 1–3 Low / 4–6 Medium / 7–9 High / 10–12 Critical

Adjust default risk up or down based on actual code evidence:
- Raise likelihood if the pattern is confirmed in code (e.g., string concatenation in SQL query found)
- Lower likelihood if a mitigation is already present (e.g., parameterized queries, input sanitization)

### Phase 4 — Attack paths

Generate 3–7 realistic multi-step attack paths based on the threat inventory. Each path chains individual threats into a coherent attacker scenario.

Format per path:

```
### [Attack Path Name]
Goal: [what the attacker achieves]
Required access: [none / authenticated user / any role]
Difficulty: [low / medium / high]

Steps:
1. [First action — threat reference]
2. [Second action — what it enables]
3. [Outcome for the attacker]

Threats: [T-list of threat IDs]
```

Examples of paths to consider (generate ones specific to this codebase):
- Account takeover chain (credential stuffing → session persistence → privilege escalation)
- Data exfiltration chain (IDOR → excessive data exposure → PII harvest)
- Admin access chain (low-privilege → vertical escalation → full control)
- Supply chain / dependency chain (known CVE → RCE or data access)
- Business logic abuse (race condition or workflow bypass → financial or data impact)

### Phase 5 — Prioritized test plan

Produce a prioritized table of tests to perform, ordered by risk score descending:

| Priority | Threat | Test | Evidence in code | Effort |
|----------|--------|------|-----------------|--------|
| 1 | [name] | [specific test to run] | [file:line if found] | quick / medium / extensive |

### Phase 6 — Output

```markdown
# Threat Model: {project name}

Date: {today}
Scope: {directories or files analysed}

---

## Application Summary

| Aspect | Finding |
|--------|---------|
| Stack | ... |
| Auth | ... |
| Roles | ... |
| Sensitive assets | ... |
| Input entry points | ... |

---

## Threat Inventory

| ID | Threat | Category | OWASP | CWE | Impact | Likelihood | Risk Score | Evidence |
|----|--------|----------|-------|-----|--------|-----------|-----------|---------|
| T1 | ... | ... | ... | ... | ... | ... | ... | [file:line or "no direct evidence"] |

---

## Attack Paths

[3–7 paths as defined above]

---

## Prioritized Test Plan

| Priority | Threat | Test | Evidence | Effort |
|----------|--------|------|---------|--------|
| 1 | ... | ... | ... | ... |

---

## Mitigations Summary

For each Critical and High threat, one concrete mitigation:

| Threat | Mitigation |
|--------|-----------|
| [T1] | [specific code change or configuration] |

---

## Skipped Categories

| Category | Reason skipped |
|---------|----------------|
| [F — File Upload] | No file upload functionality found |
```

## Rules

- Never invent threats that have no basis in the actual codebase — if a category doesn't apply, say so and skip it.
- Every threat must have a code location citation where possible, or explicitly state "no direct evidence found" when the threat is inferred from architecture rather than confirmed in code.
- The test plan must contain actionable tests — not just category labels. "Test for SQL injection" is not enough; "Send `' OR 1=1--` in the `id` parameter of `GET /api/users`" is.
- Risk scores must reflect what's in the code — adjust default scores up or down with evidence.
- Do not duplicate findings already covered by `security.md` — this report maps the full attack surface and paths; `security.md` covers confirmed code-level vulnerabilities.
