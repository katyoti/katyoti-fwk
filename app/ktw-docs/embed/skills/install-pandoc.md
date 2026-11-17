---
name: install-pandoc
description: Install Pandoc 3.11 (the version validated against ktw-docs)
category: install
advertised: false
---

# Install Pandoc — version 3.11

`ktw-docs render` is validated against the **pandoc 3.11 + typst 0.15.1** pair:
the five templates (article, report, letter, invoice, slide) and Markdown with
diagrams to PDF, DOCX, PPTX and HTML. The two are coupled — pandoc's Typst
output calls Typst functions of its own era (pandoc 3.11 emits `divider`, which
Typst 0.12 does not have) — so always install this exact pandoc version and pair
it with [install-typst](ktw-docs:/skill/install-typst).

## The declared recipes

ktw-docs and ktw-kb declare pandoc's install recipes (`describe --out json`):
winget `JohnMacFarlane.Pandoc` or scoop `pandoc` on Windows, brew `pandoc` on
macOS and Linux. A package manager installs its latest release (3.11 or later),
which `config check` reports "outdated" when it is not 3.11; the pinned
commands below install exactly 3.11. No apt recipe: Debian and Ubuntu ship an
older pandoc.

## Windows (winget)

```powershell
winget install --id JohnMacFarlane.Pandoc --version 3.11 --exact
```

If another version is already installed, uninstall it first:

```powershell
winget uninstall --id JohnMacFarlane.Pandoc
winget install --id JohnMacFarlane.Pandoc --version 3.11 --exact
```

## macOS

`brew` does not pin versions; install from the GitHub release instead.

Apple Silicon:

```bash
curl -L -o pandoc-3.11.pkg https://github.com/jgm/pandoc/releases/download/3.11/pandoc-3.11-arm64-macOS.pkg
sudo installer -pkg pandoc-3.11.pkg -target /
```

Intel:

```bash
curl -L -o pandoc-3.11.pkg https://github.com/jgm/pandoc/releases/download/3.11/pandoc-3.11-x86_64-macOS.pkg
sudo installer -pkg pandoc-3.11.pkg -target /
```

## Linux (Debian/Ubuntu)

```bash
curl -L -o pandoc-3.11.deb https://github.com/jgm/pandoc/releases/download/3.11/pandoc-3.11-1-amd64.deb
sudo dpkg -i pandoc-3.11.deb
```

For other distros download a tarball from
https://github.com/jgm/pandoc/releases/tag/3.11 and put `pandoc` on `PATH`.

## Verify

```bash
pandoc --version
```

Expected first line: `pandoc 3.11`

`ktw-docs sc config check` should then show:

```
pandoc  3.11  true
```

If the version differs, re-run the install step for your platform.
