---
name: install-docling
description: Install docling for advanced document extraction (PDF, DOCX, PPTX, HTML)
category: install
advertised: false
---

# Install docling

docling provides advanced document extraction supporting PDF, DOCX, PPTX, HTML, and more. It uses AI-based layout detection for high-quality text extraction.

ktw-kb declares it optional, with a pip recipe on every platform
(`ktw-kb describe --out json`); this page is the fallback.

## Install

Requires Python 3.10 to 3.13.

```bash
pip install docling
```

## Verify

```bash
docling --version
```
