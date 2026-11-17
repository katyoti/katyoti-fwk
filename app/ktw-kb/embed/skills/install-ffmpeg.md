---
name: install-ffmpeg
description: Install ffmpeg for audio/video processing and transcription
category: install
advertised: false
---

# Install ffmpeg

ffmpeg is needed for audio/video extraction and transcription preprocessing
only; ktw-kb declares it optional. Its install recipes (`ktw-kb describe --out
json`) are the commands below; this page is the fallback.

## Windows

```powershell
winget install --id Gyan.FFmpeg --exact
# or
scoop install ffmpeg
```

## macOS

```bash
brew install ffmpeg
```

## Linux

```bash
sudo apt-get install ffmpeg
# or
brew install ffmpeg
```

## Verify

```bash
ffmpeg -version
```
