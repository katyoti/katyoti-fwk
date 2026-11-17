---
name: install-kroki
description: Configure Kroki diagram rendering service
category: install
advertised: false
---

# Configure Kroki

Kroki renders diagrams (Mermaid, PlantUML, Graphviz, D2, etc.) to images. ktw-docs uses the public instance by default.

Kroki is a web service, not a program: ktw-docs and a publishing sidecar declare it
optional and with no install recipe.

## Default (no install needed)

ktw-docs uses `https://kroki.io` by default. No installation required.

## Self-hosted (optional)

For private diagrams or offline use, run Kroki locally:

```bash
docker run -d -p 8000:8000 yuzutech/kroki
```

Then set the URL in `DocService.yaml`:

```yaml
krokiURL: http://localhost:8000
```

## Verify

```bash
curl https://kroki.io/health
```

Expected: `{"status":"ok"}`
