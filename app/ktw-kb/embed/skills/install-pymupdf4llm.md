---
name: install-pymupdf4llm
description: Install PyMuPDF4LLM for LLM-optimized PDF extraction (Markdown output)
category: install
advertised: false
---

# Install PyMuPDF4LLM

PyMuPDF4LLM extracts PDF content as clean Markdown, optimized for LLM consumption.
ktw-kb's best PDF profile uses it and falls back to pymupdf without it, so it is
declared optional, with a pip recipe on every platform (`ktw-kb describe --out
json`); this page is the fallback.

## Install

```bash
pip install pymupdf4llm
```

## Verify

```bash
python -c "import pymupdf4llm; print('ok')"
```
