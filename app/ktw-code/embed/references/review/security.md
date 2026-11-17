# Security Review

## Purpose

Identify security vulnerabilities, weak practices, and risk areas.

## Scope

Reads: source code, configuration files, dependency manifests. Focuses exclusively on security concerns.

## Process

### 1. Injection Vulnerabilities

Check all external inputs for injection vectors:
- SQL injection (string concatenation in queries)
- Command injection (user input in shell commands)
- XSS (unescaped user input in HTML output)
- Path traversal (user input in file paths)
- Template injection (user input in template engines)
- LDAP, XML, and other injection types where applicable

### 2. Authentication & Authorization

- How are users authenticated? Is the mechanism sound?
- Are permissions checked consistently?
- Privilege escalation paths
- Session management: expiration, invalidation, token handling
- Default credentials or weak password policies

### 3. Secrets Management

- Hardcoded secrets: API keys, passwords, tokens, connection strings
- Check .gitignore for sensitive file patterns
- Secrets in logs or error messages
- Secrets loaded from environment or secret managers (not config files in repo)

### 4. Dependency Vulnerabilities

- Known vulnerabilities in dependency versions
- Abandoned packages (no maintenance)
- Supply chain risk (untrusted sources)
- Overly permissive version ranges

### 5. Data Protection

- Sensitive data stored unencrypted
- Sensitive data transmitted without encryption
- Data leaks in logs, error messages, or debug output
- Privacy concerns (personal data handling)

### 6. Configuration Security

- Default configurations — secure by default?
- Debug/development settings in production configs
- CORS, CSP, and security headers (if web)
- Permissive file permissions

### 7. OWASP Top 10 Check

Walk through the OWASP Top 10 categories applicable to this project:
- A01: Broken Access Control
- A02: Cryptographic Failures
- A03: Injection
- A04: Insecure Design
- A05: Security Misconfiguration
- A06: Vulnerable and Outdated Components
- A07: Identification and Authentication Failures
- A08: Software and Data Integrity Failures
- A09: Security Logging and Monitoring Failures
- A10: Server-Side Request Forgery

## Output Format

```markdown
# Security Review

## Critical Findings
[Issues requiring immediate attention]

| Location | Vulnerability | OWASP | Severity | Remediation |
|----------|--------------|-------|----------|-------------|
| [file:line] | [description] | [category] | critical/high/medium/low | [fix] |

## Injection Risks
[Assessment and findings]

## Authentication & Authorization
[Assessment and findings]

## Secrets Management
[Assessment — DO NOT include actual secret values]

## Dependencies
[Vulnerable or risky dependencies]

## Configuration
[Security-relevant configuration issues]

## Recommendations
[Prioritized remediation plan — critical first]
```
