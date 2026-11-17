# Configuration Explanation

## Purpose

Explain every configuration file in the project — what it controls, what each setting does, and how settings interact.

## Scope

Reads: all config files (YAML, JSON, TOML, INI, .env, XML), environment variable references in code, infrastructure-as-code files.

## Process

### 1. Configuration Inventory

Map all configuration files in the project:
- Application config (app settings, feature flags)
- Build config (webpack, vite, tsconfig, babel, etc.)
- Runtime config (Docker, Kubernetes, systemd, PM2)
- Infrastructure config (Terraform, CloudFormation, Pulumi)
- Tool config (linters, formatters, editors)
- Environment files (.env, .env.example, .env.production)

### 2. Per-File Walkthrough

For each significant config file:
- What system or service does it control?
- Walk through each significant setting:
  - What does it do?
  - What is the current value?
  - What is the default value (if different)?
  - What effect does this value have?
- Flag settings that depend on or interact with other settings
- Flag settings with security implications

### 3. Environment Variables

- List all environment variables the project expects
- For each: what it controls, whether it's required, default value
- How are env vars loaded? (dotenv, system env, secret manager)
- Are there different env files for different environments?

### 4. Configuration Layers

- How does configuration cascade? (defaults < config file < env vars < CLI args)
- Are there environment-specific overrides? (dev, staging, production)
- Is there configuration validation at startup?

### 5. Feature Flags & Toggles

- Are there feature flags or toggles?
- How are they managed? (config file, env var, remote service)
- Which features are toggleable?

## Output Format

```markdown
# Configuration

## Configuration Files
| File | Controls | Format |
|------|----------|--------|
| [file] | [what system] | [YAML/JSON/TOML/etc.] |

## [Config File Name]
| Setting | Value | Default | Purpose | Notes |
|---------|-------|---------|---------|-------|
| [key] | [current] | [default] | [what it does] | [security/interaction notes] |

## Environment Variables
| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| [VAR_NAME] | yes/no | [default] | [what it controls] |

## Configuration Cascade
[How settings override each other across environments]

## Feature Flags
| Flag | Current | Controls |
|------|---------|----------|
| [flag] | [on/off] | [what feature] |
```
