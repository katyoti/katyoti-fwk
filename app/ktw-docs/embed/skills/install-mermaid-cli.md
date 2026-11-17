---
name: install-mermaid-cli
description: Install mermaid-cli (mmdc) to render Mermaid diagrams locally, without Kroki
category: install
advertised: false
---

# Install mermaid-cli (mmdc)

`mmdc` renders Mermaid diagrams locally, using the same Mermaid engine Kroki runs
server-side. A publishing sidecar prefers it over Kroki for Mermaid because it needs no
network and Kroki's public Mermaid backend has been unreliable.

It ships as an npm package and bundles a headless Chromium via Puppeteer, so the
first install is large (~300 MB) and the first render takes a second or two
longer than later ones.

A publishing sidecar declares it optional, with a bun install recipe on every
platform (its `describe --out json`); this page is the fallback.

## All platforms

With [bun](ktw-docs:/skill/install-bun) (the declared recipe):

```bash
bun install -g @mermaid-js/mermaid-cli
```

Or with npm (requires Node.js 18+, `node --version`):

```bash
npm install -g @mermaid-js/mermaid-cli
```

## Verify

```bash
mmdc --version
```

Then render a throwaway diagram:

```bash
printf 'graph TD\n  A[Start] --> B[End]\n' > t.mmd && mmdc -i t.mmd -o t.svg
```

`t.svg` should exist and be a few kilobytes.

## If `mmdc` is not on PATH

npm's global bin directory is often missing from a non-login shell's PATH
(bun's is `~/.bun/bin`, which bun's installer adds to PATH). Find npm's with
`npm bin -g`, then either add that directory to PATH or point the publishing sidecar
straight at the binary:

```bash
export KTW_MERMAID_CLI=/full/path/to/mmdc      # macOS / Linux
setx KTW_MERMAID_CLI "%APPDATA%\npm\mmdc.cmd"  # Windows
```

## Choosing the renderer

The publishing sidecar picks the Mermaid renderer from the `renderer:` field on the
`mermaid` entry in `ConfluenceDiagrams.yaml`:

| value | behaviour |
|---|---|
| `auto` (default) | mermaid-cli when installed, else Kroki |
| `local` | require mermaid-cli — fails with this install hint if missing |
| `kroki` | always use the Kroki service |

Every other diagram language (PlantUML, Graphviz, D2, …) always uses Kroki —
mermaid-cli only renders Mermaid.

## Troubleshooting

**Chromium fails to launch** (common on headless Linux/CI): install the shared
libraries Puppeteer needs, or point it at a system Chromium with
`PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium`.

**Renders are slow.** Each invocation boots Chromium (~1.7 s). Rendered diagrams
are cached under the workspace's `.confluence/cache/diagrams/`, so only diagrams
whose source actually changed pay that cost.
