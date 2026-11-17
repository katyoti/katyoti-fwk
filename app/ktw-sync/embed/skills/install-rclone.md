---
name: install-rclone
description: Install rclone for cloud storage synchronization
category: install
advertised: false
---

# Install rclone

rclone is required by `ktw-sync` for cloud storage synchronization (OneDrive, SharePoint, S3, etc.).

ktw-sync declares its install recipes (`ktw-sync describe --out json`): the
package-manager commands below; an installer runs them, this page is the
fallback.

## Windows

```powershell
winget install --id Rclone.Rclone --exact
# or
scoop install rclone
```

## macOS

```bash
brew install rclone
```

## Linux

```bash
sudo apt-get install rclone
# or
brew install rclone
# or rclone's own installer (the latest release; apt's may lag behind)
curl https://rclone.org/install.sh | sudo bash
```

## Configure

```bash
rclone config
```

Follow the interactive setup to configure cloud providers.

## Verify

```bash
rclone version
```
