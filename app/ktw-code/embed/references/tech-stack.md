# Tech Stack Explanation

## Purpose

Explain every technology, language, framework, library, and runtime the project uses — what it is, why it's here, and how it fits.

## Scope

Reads: package manifests (package.json, requirements.txt, go.mod, Cargo.toml, pom.xml, etc.), lock files, build configs, Dockerfile, CI configs, runtime configs.

## Process

### 1. Languages & Runtimes

- Which programming languages are used?
- Which runtime versions are required?
- Are there multiple languages? (e.g., TypeScript frontend + Python backend)

### 2. Frameworks

- Which frameworks are in use? (web, testing, ORM, CLI, etc.)
- What version of each?
- What role does each framework play?

### 3. Libraries & Dependencies

For each significant dependency:
- What does it do?
- Why is it here? (what feature/capability does it provide)
- Is it a direct dependency or transitive?
- Production vs development dependency?

Group by role:
- Core functionality
- HTTP / networking
- Database / storage
- Authentication / security
- Testing
- Build / tooling
- UI / frontend
- Utilities

### 4. Build & Package Tools

- Package manager (npm, pip, cargo, maven, etc.)
- Build tool (webpack, vite, tsc, make, gradle, etc.)
- Bundling, transpilation, compilation steps

### 5. Infrastructure

- Containerization (Docker, Podman)
- Orchestration (Kubernetes, Docker Compose)
- Cloud services referenced in config
- Database systems

## Output Format

```markdown
# Tech Stack

## Languages & Runtimes
| Language | Version | Role |
|----------|---------|------|
| [lang] | [version] | [primary/secondary, what part] |

## Frameworks
| Framework | Version | Purpose |
|-----------|---------|---------|
| [framework] | [version] | [what it does in this project] |

## Key Dependencies
### [Category]
| Library | Version | Purpose |
|---------|---------|---------|
| [lib] | [version] | [why it's here] |

## Build Toolchain
[Package manager -> build tool -> output]

## Infrastructure
[Containers, databases, cloud services]

## Version Constraints
[Any notable version pinning, compatibility requirements, or deprecation risks]
```
