---
name: install-python
description: Install Python 3, which installs and runs ktw-kb's PDF extraction
category: install
advertised: false
---

# Install Python

Python 3 runs ktw-kb's PDF extraction (pymupdf) and installs it: every pip
recipe runs as `python -m pip install --user`. ktw-kb declares it required,
with a recipe on every platform (`ktw-kb describe --out json`), and an
installer installs it before the pip packages; this page is the fallback.

## Install

| Platform | Command |
|---|---|
| Windows | `winget install --id Python.Python.3.12 --exact --scope user` (or `scoop install python`) |
| macOS | `brew install python@3.12` |
| Linux | `sudo apt-get install -y python3-pip` (brings python3) |

On Windows, the `python` the Microsoft Store puts on PATH is a stub that
installs nothing by itself: it answers no version, and the check does not take
it for Python.

## Verify

```bash
python --version
python -m pip --version
```
