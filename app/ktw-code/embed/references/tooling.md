# Tooling & Dev Environment Explanation

## Purpose

Explain the development tools, CI/CD pipeline, testing setup, linting, and AI/LLM integration the project uses.

## Scope

Reads: CI/CD configs (.github/workflows, .gitlab-ci.yml, Jenkinsfile), linter/formatter configs (.eslintrc, .prettierrc, ruff.toml, .editorconfig), test configs, Makefile/scripts, and LLM tool configs (.cursor/, .claude/, CLAUDE.md, .aider*, copilot settings).

## Process

### 1. Development Tools

- Editor/IDE configurations present (.vscode/, .idea/, .editorconfig)
- Linters and formatters (ESLint, Prettier, Black, Ruff, etc.)
- Pre-commit hooks (husky, pre-commit, lint-staged)
- Debug configurations

### 2. Testing Setup

- Test framework(s) in use (Jest, pytest, Go test, JUnit, etc.)
- Test organization (unit, integration, e2e — how are they separated?)
- Test runner commands
- Coverage configuration
- Fixtures, mocks, test utilities

### 3. CI/CD Pipeline

- What CI system is used? (GitHub Actions, GitLab CI, Jenkins, etc.)
- Pipeline stages (build, test, lint, deploy)
- What triggers the pipeline? (push, PR, tag, schedule)
- Deployment targets and strategies
- Environment-specific configurations

### 4. Scripts & Automation

- Package scripts (npm scripts, Makefile targets, shell scripts)
- Code generation tools
- Database migration tools
- Documentation generation

### 5. AI/LLM Integration

Check for and explain any AI-assisted development tools:

- **Claude Code**: `.claude/` directory, `CLAUDE.md` file — project instructions, skills, settings
- **Cursor**: `.cursor/` directory, `.cursorrules` — AI rules and configuration
- **Aider**: `.aider*` files — AI coding assistant configuration
- **GitHub Copilot**: `.github/copilot*` — Copilot configuration
- **Other**: any `.ai/`, `.llm/`, or similar directories

For each found:
- What is it configured to do?
- What rules/instructions are set?
- What tools/skills are enabled?

### 6. Code Quality Gates

- What checks must pass before merge?
- Are there required reviewers?
- Is there branch protection?
- Are there quality thresholds (coverage minimums, lint rules)?

## Output Format

```markdown
# Tooling & Dev Environment

## Development Tools
| Tool | Config File | Purpose |
|------|-----------|---------|
| [tool] | [config] | [what it does] |

## Testing
- **Framework**: [name]
- **Organization**: [how tests are structured]
- **Commands**: [how to run tests]
- **Coverage**: [configured? threshold?]

## CI/CD
- **System**: [GitHub Actions / GitLab CI / etc.]
- **Triggers**: [when it runs]
- **Stages**:
  1. [stage]: [what it does]
  2. [stage]: [what it does]

## Scripts
| Command | Purpose |
|---------|---------|
| [command] | [what it does] |

## AI/LLM Tools
| Tool | Config | Purpose |
|------|--------|---------|
| [tool] | [files] | [how it's configured] |

## Quality Gates
[What must pass before code can be merged]
```
