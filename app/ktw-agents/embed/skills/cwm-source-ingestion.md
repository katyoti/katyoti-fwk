---
name: cwm-source-ingestion
description: >-
  Anchor outside documents in CWM through the sources service
  (source-ingest, source-reanchor, source-stale): derived-from traceability and drift detection.
iconName: anchor
icon: "⚓"
hashtags: [cwm, source, ingest, anchor, traceability]
category: cwm
advertised: true
---

# Anchoring External Sources

CWM's traceability invariant: every object derived from an external source MUST carry a `derived-from` edge pointing at a `source-document` object. The sources service records the anchor, detects when the external content drifts and reports the downstream objects to recheck.

## Why anchors matter

Without anchors:

- An agent reads "RFC 7396 says PATCH semantics …" but the linked spec moves to a new section — the agent's `decision` object is silently stale.
- An agent extracts a rule from internal doc v3, the doc rolls to v4 with a different rule — nobody notices.

With anchors:

- The `source-document` object carries a `content_hash` + a `content_snapshot` of the relevant section.
- Downstream `decision` / `rule` objects carry a `derived-from` edge to the `source-document` object (the section it anchors is the document's `section_path`: one source-document per anchored section).
- The `sources` plugin (`plugins: [sources]`) does the anchoring: `source-ingest` records the section, `source-reanchor` reads it again, `source-stale` lists what drifted. **Never compute a hash or a snapshot by hand** — the service computes both with the one function the reanchor uses, so a source ingested and re-anchored unchanged answers `match`.

## The ingest flow

1. **Ingest the section** — the service reads it, hashes it, snapshots it and writes the `source-document` object:
   ```
   source-ingest(
     source_doc_id="<module>.<local>",          # created when absent, anchored afresh when present
     source_uri="file://./docs/spec.md",
     section_path=["Authentication", "Rate Limits"],   # heading breadcrumb; [] = the whole document
     parent="<parent-id>",                      # on create; empty = the module
     ingested_by="<persona>"
   )
   ```
   - `file://` sources are read **only inside the project**: `file://./docs/spec.md` is relative to the project root; a path elsewhere on the disk, a `..` escape or a symlink pointing out is refused.
   - Any other scheme (`https://`, `confluence://`, …) has no fetcher: read the document yourself and pass it as `new_content`; the service still extracts, hashes and snapshots it.
   - A `section_path` not found in the document is refused — fix the breadcrumb, do not create the object by hand.
   - The answer gives `content_hash` and `snapshot_chars` (the snapshot keeps at most 3,500 characters).
2. **Create derived objects** carrying a `derived-from` edge to the source-document:
   ```
   cwm-object-create(
     module="<module>",
     object={
       type: "decision",
       name: "...",
       parent: "<parent-id>",
       body: ...,
       edges: [{kind: "derived-from", to: "<source-doc-id>"}]
     }
   )
   ```
3. **Reanchor** on demand, then **list what is stale**:
   ```
   source-reanchor(source_doc_id="<id>", update_on_drift=false)
   source-stale(module="<module>")
   ```

## What reanchor does

`source-reanchor` returns:

- `status`: `match` (content unchanged), `drift` (content changed, snapshot stale), `missing` (section not found anymore).
- `stale_item_ids` / `missing_item_ids`: the derived objects to recheck — reported, nothing is written on them.

It records the outcome on the source-document (`anchor_status` `valid`, `stale` or `missing`). With `update_on_drift=true` it records the new snapshot + hash and the document stays `valid`. The downstream objects still need reviewing — automatic regeneration would mask real spec changes.

## Surfacing stale anchors

`source-stale(module)` answers every source-document of the module whose last reanchor left it `stale` or `missing`, each with its `source_uri`, `section_path` and `derived_item_ids` — the objects to recheck. It reads only; a document is marked by a reanchor, so reanchor first when the sources may have moved.

Agents that operate on an object with stale anchors should refuse to advance lifecycle gates until the anchors are reconciled: review the derived objects, then `source-ingest` the section again (or `source-reanchor` with `update_on_drift=true`) to record the new anchor.

## The substrate-vs-method-pack boundary

The sources extension ships the `source-document` object type and the ingest / reanchor / stale mechanism. Method packs ship specialized derived types: `safe-requirement` (ktw-safe), `book-quote` (future ktw-book), etc. Every method pack reuses the same anchor protocol and the same service.

## What to read

- [cwm-knowledge-capture](cwm-knowledge-capture.md) — choosing between `source-document` (snapshot) and `document` (workspace-owned prose).
- [cwm-records-not-docs](cwm-records-not-docs.md) — source-document is one of the `yaml-with-body` types that carries prose legitimately.
