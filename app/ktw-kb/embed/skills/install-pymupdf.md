---
name: install-pymupdf
description: Install PyMuPDF for PDF text extraction
category: install
advertised: false
---

# Install PyMuPDF

PyMuPDF extracts text from PDF files for knowledge base indexing — the PDF
extraction every profile falls back to, so ktw-kb declares it required, with a
pip recipe on every platform (`ktw-kb describe --out json`); this page is the
fallback.

## Install

```bash
pip install pymupdf
```

## Verify

```bash
python -c "import pymupdf; print(pymupdf.__version__)"
```
