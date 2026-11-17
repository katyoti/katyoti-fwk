---
name: install-office-com
description: Install pywin32 for Office COM automation (Windows + Office required)
category: install
advertised: false
---

# Install Office COM Support

pywin32 enables COM automation for extracting text from Word/Excel/PowerPoint files on Windows.

**Requirements**: Windows + Microsoft Office installed.

ktw-kb declares it optional, with a pip recipe (`pywin32`) on Windows only:
Office itself is not installable by a recipe.

## Install

```bash
pip install pywin32
```

## Verify

```bash
python -c "import win32com.client; print('ok')"
```

## Note

This only works on Windows with Microsoft Office installed. On macOS/Linux, use alternative extractors (pymupdf for PDF, docling for DOCX).
