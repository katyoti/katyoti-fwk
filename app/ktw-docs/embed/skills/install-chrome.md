---
name: install-chrome
description: Install Chrome/Chromium for slide export and browser-based rendering
category: install
advertised: false
---

# Install Chrome / Chromium

Chrome or Chromium is used for browser-based rendering and export; ktw-docs
declares it optional. Its install recipes (`describe --out json`) are the
commands below; this page is the fallback.

## Windows

Chrome is usually already installed. If not:

```powershell
winget install --id Google.Chrome --exact
# or
scoop bucket add extras
scoop install extras/googlechrome
```

## macOS

```bash
brew install --cask google-chrome
```

Or install Chromium:

```bash
brew install --cask chromium
```

## Linux

```bash
# Chromium — Debian
sudo apt-get install chromium
# Chromium — Ubuntu
sudo apt-get install chromium-browser

# Or Google Chrome
wget https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb
sudo dpkg -i google-chrome-stable_current_amd64.deb
```

## Verify

```bash
google-chrome --version
# or
chromium --version
```
