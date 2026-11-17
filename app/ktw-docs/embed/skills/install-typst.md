---
name: install-typst
description: Install Typst 0.15.1 (the version validated against ktw-docs)
category: install
advertised: false
---

# Install Typst — version 0.15.1

`ktw-docs render` uses Typst as the PDF engine, for the default layout and for
the `.typ` templates (article, report, letter, invoice, slide). It is validated
against the **pandoc 3.11 + typst 0.15.1** pair. The two are coupled — pandoc's
Typst output calls Typst functions of its own era (pandoc 3.11 emits `divider`,
which Typst 0.12 does not have) — so always install this exact typst version and
pair it with [install-pandoc](ktw-docs:/skill/install-pandoc).

The templates' default font is Libertinus Serif, which Typst bundles.

## The declared recipes

ktw-docs declares typst's install recipes (`describe --out json`): winget
`Typst.Typst` or scoop `typst` on Windows, brew `typst` on macOS and Linux. A
package manager installs its latest release (0.15.1 or later), which `config
check` reports "outdated" when it is not 0.15.1; the pinned commands below
install exactly 0.15.1.

## Windows (winget)

```powershell
winget install --id Typst.Typst --version 0.15.1 --exact
```

If another version is already installed, uninstall it first:

```powershell
winget uninstall --id Typst.Typst
winget install --id Typst.Typst --version 0.15.1 --exact
```

A `typst` installed by other means (e.g. `~/.local/bin/typst`) may come first
on `PATH`; check with `where typst` and remove the stale one.

## macOS

`brew` does not pin versions; install from the GitHub release instead.

Apple Silicon:

```bash
curl -L -o typst-0.15.1.tar.xz https://github.com/typst/typst/releases/download/v0.15.1/typst-aarch64-apple-darwin.tar.xz
tar -xJf typst-0.15.1.tar.xz
sudo mv typst-aarch64-apple-darwin/typst /usr/local/bin/typst
```

Intel:

```bash
curl -L -o typst-0.15.1.tar.xz https://github.com/typst/typst/releases/download/v0.15.1/typst-x86_64-apple-darwin.tar.xz
tar -xJf typst-0.15.1.tar.xz
sudo mv typst-x86_64-apple-darwin/typst /usr/local/bin/typst
```

## Linux

```bash
curl -L -o typst-0.15.1.tar.xz https://github.com/typst/typst/releases/download/v0.15.1/typst-x86_64-unknown-linux-musl.tar.xz
tar -xJf typst-0.15.1.tar.xz
sudo mv typst-x86_64-unknown-linux-musl/typst /usr/local/bin/typst
```

For other architectures pick the right artifact from
https://github.com/typst/typst/releases/tag/v0.15.1

## Verify

```bash
typst --version
```

Expected first line: `typst 0.15.1 (9dfd3a08)`

`ktw-docs sc config check` should then show:

```
typst  0.15.1 (9dfd3a08)  true
```

If the version differs, re-run the install step for your platform.
