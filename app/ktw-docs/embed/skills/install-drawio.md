---
name: install-drawio
description: Install draw.io desktop for diagram editing and export
category: install
advertised: false
---

# Install draw.io Desktop

draw.io desktop is used for DrawIO file export and diagram rendering, only
for `.drawio` sources: ktw-docs and a publishing sidecar declare it optional. Their
install recipes (`describe --out json`) cover Windows and macOS; on Linux this
page is the only way.

## Windows

Attention to use scope user to install the app for the current user. Else you need to be admin to install the app for all users, and most of the time company policy allows no admin rights.

```powershell
winget install -e --id JGraph.Draw --scope user
```

Or with scoop:

```powershell
scoop bucket add extras
scoop install extras/draw.io
```

Or download from https://github.com/jgraph/drawio-desktop/releases

## macOS

```bash
brew install --cask drawio
```

## Linux

Download the AppImage, .deb or .rpm for your architecture from
https://github.com/jgraph/drawio-desktop/releases — there is no apt or brew
package, hence no declared recipe.

## Verify

draw.io should be available in your Applications or on PATH as `drawio`.
