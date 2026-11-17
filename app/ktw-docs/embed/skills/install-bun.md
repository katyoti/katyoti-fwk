---
name: install-bun
description: Install bun JavaScript/TypeScript runtime (needed for ELK diagram layout)
category: install
advertised: false
---

# Install bun

bun is needed for the architecture diagram pipeline (ELK layout engine) only;
ktw-docs declares it optional. It also installs
[mermaid-cli](ktw-docs:/skill/install-mermaid-cli).

ktw-docs declares these install recipes (`ktw-docs describe --out json`); an
installer runs them, this page is the fallback.

## Windows

```powershell
winget install --id Oven-sh.Bun --exact
# or
scoop install bun
# or bun's own installer
powershell -c "irm bun.sh/install.ps1 | iex"
```

## macOS / Linux

```bash
brew install bun
# or bun's own installer
curl -fsSL https://bun.sh/install | bash
```

## Verify

```bash
bun --version
```

Expected: bun 1.0 or later.
