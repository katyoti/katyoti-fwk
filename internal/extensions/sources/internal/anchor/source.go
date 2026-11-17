// SPDX-License-Identifier: Apache-2.0

// Package anchor computes a source-document's anchor: fetch the source it
// names, extract the anchored section, compare the hash with the recorded
// one. It reads and writes no object — the sources service loads the
// source-document object through CWM, hands its fields here, stores the
// fields it gets back and reports the objects that carry a `derived-from`
// edge to it.
package anchor

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Service holds the fetchers, one per URI scheme.
type Service struct {
	fetchers *fetcherRegistry
}

// New returns a Service with the `file` fetcher registered.
func New() *Service {
	svc := &Service{fetchers: newFetcherRegistry()}
	svc.fetchers.register("file", FileFetcher{})
	return svc
}

// RegisterFetcher adds (or replaces) the fetcher for a URI scheme.
func (s *Service) RegisterFetcher(scheme string, f Fetcher) {
	s.fetchers.register(scheme, f)
}

// Result is one reanchor's outcome.
type Result struct {
	// Body is the source-document's body after the reanchor: the recorded
	// fields patched, every other field kept.
	Body json.RawMessage
	// Status is match | drift | missing; AnchorStatus is what the body now
	// records (valid | stale | missing).
	Status, AnchorStatus string
	OldHash, NewHash     string
}

// Reanchor recomputes a source-document body's anchor. newContent, when set,
// is the source's content (the caller fetched it); otherwise the source is
// fetched by the scheme of its source_uri. On drift the body is marked stale,
// or, with updateOnDrift, re-snapshotted and marked valid.
func (s *Service) Reanchor(ctx stdctx.Context, raw json.RawMessage, newContent string, updateOnDrift bool) (Result, error) {
	body, err := decodeSourceDocBody(raw)
	if err != nil {
		return Result{}, err
	}
	res := Result{OldHash: body.ContentHash}

	content, err := s.getSourceContent(ctx, body.SourceURI, newContent)
	if err != nil {
		return Result{}, err
	}
	anc, found := anchorSection(content, body.SectionPath)
	switch {
	case !found:
		res.Status = "missing"
		body.AnchorStatus = AnchorStatusMissing
	default:
		res.NewHash = anc.Hash
		switch {
		case res.NewHash == res.OldHash:
			res.Status = "match"
			body.AnchorStatus = AnchorStatusValid
		case updateOnDrift:
			res.Status = "drift"
			body.ContentHash = anc.Hash
			body.ContentSnapshot = anc.Snapshot
			body.AnchorStatus = AnchorStatusValid
		default:
			res.Status = "drift"
			body.AnchorStatus = AnchorStatusStale
		}
	}
	body.LastReanchorAt = time.Now().UTC()
	res.AnchorStatus = body.AnchorStatus
	if res.Body, err = patchBody(raw, body); err != nil {
		return Result{}, err
	}
	return res, nil
}

// IngestResult is one ingest's outcome: the body to store and its anchor.
type IngestResult struct {
	Body json.RawMessage
	Anchor
}

// Ingest anchors a source for the first time, or anchors it afresh: the
// section is read and hashed by the same anchorSection Reanchor uses, so the
// first reanchor of an unchanged source answers `match`. raw is the existing
// body (nil for a new document); its other fields are kept.
func (s *Service) Ingest(ctx stdctx.Context, raw json.RawMessage, uri string, sectionPath []string, newContent, ingestedBy string) (IngestResult, error) {
	if uri == "" {
		return IngestResult{}, fmt.Errorf("%w: source_uri required", api.ErrInvalidInput)
	}
	body, err := decodeSourceDocBody(raw)
	if err != nil {
		return IngestResult{}, err
	}
	content, err := s.getSourceContent(ctx, uri, newContent)
	if err != nil {
		return IngestResult{}, err
	}
	anc, found := anchorSection(content, sectionPath)
	if !found {
		return IngestResult{}, fmt.Errorf("%w: section %q not found in %s", api.ErrInvalidInput, sectionPath, uri)
	}
	body.SourceURI = uri
	body.SectionPath = sectionPath
	body.ContentHash = anc.Hash
	body.ContentSnapshot = anc.Snapshot
	body.IngestedAt = time.Now().UTC()
	if ingestedBy != "" {
		body.IngestedBy = ingestedBy
	}
	body.AnchorStatus = AnchorStatusValid
	var drop []string
	if len(sectionPath) == 0 {
		drop = append(drop, "section_path")
	}
	out, err := patchBody(raw, body, drop...)
	if err != nil {
		return IngestResult{}, err
	}
	return IngestResult{Body: out, Anchor: anc}, nil
}

// Anchor is what a section is recorded by: the hash of its normalized text and
// a snapshot of its text.
type Anchor struct {
	Hash     string
	Snapshot string
}

// anchorSection extracts the section at path and computes its anchor — the ONE
// computation behind Ingest and Reanchor. found is false when the section is
// not in the content.
func anchorSection(content string, path []string) (Anchor, bool) {
	extracted, found := extractMarkdownSection(content, path)
	if !found {
		return Anchor{}, false
	}
	return Anchor{Hash: hashOf(normalizeText(extracted)), Snapshot: truncate(extracted, maxSnapshotChars)}, true
}

// maxSnapshotChars caps the recorded content_snapshot, in characters (runes).
const maxSnapshotChars = 3500

// getSourceContent returns the bytes to compare against: newContent when the
// caller supplied it, else what the scheme's fetcher returns.
func (s *Service) getSourceContent(ctx stdctx.Context, uri, newContent string) (string, error) {
	if newContent != "" {
		return newContent, nil
	}
	if uri == "" {
		return "", fmt.Errorf("%w: source-document has empty source_uri", api.ErrInvalidInput)
	}
	scheme := schemeOf(uri)
	if scheme == "" {
		return "", fmt.Errorf("%w: source_uri %q has no scheme", api.ErrInvalidInput, uri)
	}
	fetcher, ok := s.fetchers.lookup(scheme)
	if !ok {
		return "", fmt.Errorf("%w: no fetcher registered for scheme %q (uri=%q)", api.ErrInvalidInput, scheme, uri)
	}
	data, err := fetcher.Fetch(ctx, uri)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// patchBody overlays the typed fields onto the original body, keeping any
// field the typed view does not know.
// Keys in drop are removed from the body.
func patchBody(raw json.RawMessage, body SourceDocBody, drop ...string) (json.RawMessage, error) {
	doc := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("sources: decode body: %w", err)
		}
	}
	patch, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("sources: marshal body: %w", err)
	}
	var patchMap map[string]any
	if err := json.Unmarshal(patch, &patchMap); err != nil {
		return nil, fmt.Errorf("sources: re-decode body: %w", err)
	}
	// A zero time marshals as "0001-01-01T00:00:00Z": never record one.
	if body.IngestedAt.IsZero() {
		delete(patchMap, "ingested_at")
	}
	if body.LastReanchorAt.IsZero() {
		delete(patchMap, "last_reanchor_at")
	}
	for k, v := range patchMap {
		doc[k] = v
	}
	for _, k := range drop {
		delete(doc, k)
	}
	return json.Marshal(doc)
}

// truncate returns s clipped to at most maxLen characters (runes): a cut never
// falls inside a UTF-8 sequence.
func truncate(s string, maxLen int) string {
	n := 0
	for i := range s {
		if n == maxLen {
			return s[:i]
		}
		n++
	}
	return s
}

// CutSnapshot shortens a body's content_snapshot by at least over bytes (half
// of it when over is unknown, 0 or less), on a rune boundary, so the document
// fits its type's size budget. The hash is the full section's and is kept: the
// snapshot is what a reader sees, the hash is what drift is decided on. ok is
// false when there is no snapshot left to cut (the field is required, so it
// is never emptied).
func CutSnapshot(raw json.RawMessage, over int) (json.RawMessage, bool, error) {
	body, err := decodeSourceDocBody(raw)
	if err != nil {
		return nil, false, err
	}
	snap := body.ContentSnapshot
	if snap == "" {
		return raw, false, nil
	}
	target := len(snap) / 2
	if over > 0 {
		target = len(snap) - over - snapshotCutSlack
	}
	for target > 0 && !utf8.RuneStart(snap[target]) {
		target--
	}
	if target <= 0 {
		return raw, false, nil // the snapshot is required: what is over budget is the rest
	}
	body.ContentSnapshot = snap[:target]
	out, err := patchBody(raw, body)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// snapshotCutSlack is cut beyond the overshoot: the encoded file does not
// shrink byte for byte with its snapshot (a quoting or line style may change).
const snapshotCutSlack = 16
