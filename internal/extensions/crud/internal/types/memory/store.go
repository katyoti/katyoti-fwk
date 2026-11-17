// SPDX-License-Identifier: Apache-2.0

// Package memory is the first concrete CRUD backend — an in-process
// connection scoped to its named instance. Two `memory` connections
// declared in Connections.yaml share NO state; the connection name is
// the bucket boundary.
//
// For session-scoped memory (one bucket per session, derived from ctx
// rather than from the connection name) see crud/types/session_memory.
//
// Watch is honoured. Change events publish through the injected
// EventBus on the canonical subject built by crud/api.EventSubject.
// Subscribers attach via eventbus.EventBus.Subscribe — once
// ConnectionService routes the topic through NATS (via the
// MessageBus interface) the same subject works across the wire.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	crudapi "github.com/katyoti/katyoti-fwk/internal/extensions/crud/api"
)

// MemoryStore implements api.CRUDService over an in-process map.
// (type, id) → Record buckets behind a single RWMutex. Safe for
// concurrent Load / List with a single writer at a time on Save /
// Delete; the RWMutex keeps reads parallel.
type MemoryStore struct {
	name     string              // connection name — used as the EventSubject `connection` token
	eventBus eventbus.EventBus   // for Watch event emission (use eventbus.NopEventBus to disable)
	nowFn    func() time.Time    // injectable clock; overridden in tests
	idFn     func(string) string // injectable ID generator; gets the type name as input

	mu      sync.RWMutex
	records map[string]map[string]crudapi.Record // type → id → Record
	seq     uint64
}

// NewMemoryStore constructs a MemoryStore. Pass `eventbus.NopEventBus{}`
// when Watch isn't needed (Capabilities.Watch.Supported still reads
// `true` — callers that don't subscribe simply never get events).
func NewMemoryStore(name string, bus eventbus.EventBus) *MemoryStore {
	s := &MemoryStore{
		name:     name,
		eventBus: bus,
		nowFn:    time.Now,
		records:  make(map[string]map[string]crudapi.Record),
	}
	s.idFn = s.defaultID
	return s
}

func (s *MemoryStore) defaultID(typeName string) string {
	s.seq++
	return fmt.Sprintf("%s-%d", typeName, s.seq)
}

// ----------------------------------------------------------------------
// CRUDService implementation
// ----------------------------------------------------------------------

// parseRecordURI parses a URI as a flat record reference (Load /
// Delete shape). Returns (type, id, nil) when the URI is
// `<type>/<id>`, an error otherwise.
func (s *MemoryStore) parseRecordURI(uri crudapi.URI) (string, string, error) {
	p, err := crudapi.ParseURI(uri)
	if err != nil {
		return "", "", err
	}
	t, id, err := p.ScopedID()
	if err != nil {
		return "", "", err
	}
	if id == "" {
		return "", "", fmt.Errorf("URI %q has no ID segment (record operations require <type>/<id>): %w", p.Path, crudapi.ErrInvalidInput)
	}
	return t, id, nil
}

// parseCollectionURI parses a URI as a collection reference (List /
// Save-with-server-id shape). Returns (type, "", nil) for
// `<type>` OR (type, id, nil) for `<type>/<id>` — the caller decides
// whether ID is required by method (Save accepts empty ID; List
// rejects non-empty).
func (s *MemoryStore) parseCollectionURI(uri crudapi.URI) (string, string, error) {
	p, err := crudapi.ParseURI(uri)
	if err != nil {
		return "", "", err
	}
	return p.ScopedID()
}

func (s *MemoryStore) Load(_ context.Context, in crudapi.LoadInput) (crudapi.LoadOutput, error) {
	typ, id, err := s.parseRecordURI(in.URI)
	if err != nil {
		return crudapi.LoadOutput{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket := s.records[typ]
	if bucket == nil {
		return crudapi.LoadOutput{}, fmt.Errorf("type %q id %q: %w", typ, id, crudapi.ErrNotFound)
	}
	rec, ok := bucket[id]
	if !ok {
		return crudapi.LoadOutput{}, fmt.Errorf("type %q id %q: %w", typ, id, crudapi.ErrNotFound)
	}
	return crudapi.LoadOutput{Record: rec}, nil
}

func (s *MemoryStore) Save(ctx context.Context, in crudapi.SaveInput) (crudapi.SaveOutput, error) {
	typ, id, err := s.parseCollectionURI(in.URI)
	if err != nil {
		return crudapi.SaveOutput{}, err
	}
	if typ == "" {
		return crudapi.SaveOutput{}, fmt.Errorf("URI must include a type segment: %w", crudapi.ErrInvalidInput)
	}
	now := s.nowFn()

	s.mu.Lock()
	bucket := s.records[typ]
	if bucket == nil {
		bucket = make(map[string]crudapi.Record)
		s.records[typ] = bucket
	}
	if id == "" {
		id = s.idFn(typ)
	}
	old, existed := bucket[id]

	rec := crudapi.Record{
		Type:      typ,
		ID:        id,
		Body:      in.Body,
		CreatedAt: now,
		UpdatedAt: now,
		Version:   old.Version + 1,
	}
	if existed {
		rec.CreatedAt = old.CreatedAt
	}
	bucket[id] = rec
	s.mu.Unlock()

	// Emit AFTER releasing the lock — synchronous handlers running under
	// the store's mutex would deadlock on any subscriber that called
	// back in.
	kind := crudapi.WatchKindCreated
	var oldPtr *crudapi.Record
	if existed {
		kind = crudapi.WatchKindUpdated
		oldCopy := old
		oldPtr = &oldCopy
	}
	recCopy := rec
	s.emit(ctx, kind, typ, id, &recCopy, oldPtr)
	return crudapi.SaveOutput{Record: rec}, nil
}

func (s *MemoryStore) Delete(ctx context.Context, in crudapi.DeleteInput) (crudapi.DeleteOutput, error) {
	typ, id, err := s.parseRecordURI(in.URI)
	if err != nil {
		return crudapi.DeleteOutput{}, err
	}
	s.mu.Lock()
	bucket := s.records[typ]
	if bucket == nil {
		s.mu.Unlock()
		return crudapi.DeleteOutput{}, fmt.Errorf("type %q id %q: %w", typ, id, crudapi.ErrNotFound)
	}
	old, ok := bucket[id]
	if !ok {
		s.mu.Unlock()
		return crudapi.DeleteOutput{}, fmt.Errorf("type %q id %q: %w", typ, id, crudapi.ErrNotFound)
	}
	delete(bucket, id)
	s.mu.Unlock()

	oldCopy := old
	s.emit(ctx, crudapi.WatchKindDeleted, typ, id, nil, &oldCopy)
	return crudapi.DeleteOutput{OK: true}, nil
}

func (s *MemoryStore) List(_ context.Context, in crudapi.ListInput) (crudapi.ListOutput, error) {
	p, err := crudapi.ParseURI(in.URI)
	if err != nil {
		return crudapi.ListOutput{}, err
	}
	typ := p.Type()
	if typ == "" {
		return crudapi.ListOutput{}, fmt.Errorf("URI %q has no type segment: %w", p.Path, crudapi.ErrInvalidInput)
	}
	// Reject record URIs (List takes a collection URI per method semantics).
	if len(p.Segments) > 1 {
		return crudapi.ListOutput{}, fmt.Errorf("URI %q is a record URI; List takes a collection URI (drop the trailing /id): %w", p.Path, crudapi.ErrInvalidInput)
	}

	// Validate filter ops up front — invalid op should surface as
	// ErrInvalidInput rather than silently match-nothing.
	for field, fv := range in.Filters {
		if !supportedOp(fv.Op) {
			return crudapi.ListOutput{}, fmt.Errorf("filter %q: unsupported op %q: %w", field, fv.Op, crudapi.ErrInvalidInput)
		}
	}

	s.mu.RLock()
	bucket := s.records[typ]
	matched := make([]crudapi.Record, 0, len(bucket))
	for _, rec := range bucket {
		ok, err := matchFilters(rec, in.Filters)
		if err != nil {
			s.mu.RUnlock()
			return crudapi.ListOutput{}, err
		}
		if ok {
			matched = append(matched, rec)
		}
	}
	s.mu.RUnlock()

	if len(in.OrderBy) > 0 {
		if err := sortRecords(matched, in.OrderBy); err != nil {
			return crudapi.ListOutput{}, err
		}
	} else {
		// Stable default order — by ID ascending. Deterministic for
		// tests and pagination correctness.
		sort.Slice(matched, func(i, j int) bool { return matched[i].ID < matched[j].ID })
	}

	if in.Distinct {
		matched = dedupRecords(matched)
	}

	total := int64(len(matched))
	limit := in.Limit
	if limit <= 0 {
		limit = 100 // see Capabilities().Pagination.DefaultLimit
	}
	offset := in.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > len(matched) {
		offset = len(matched)
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	page := append([]crudapi.Record(nil), matched[offset:end]...)

	return crudapi.ListOutput{
		Records: page,
		Pagination: crudapi.Pagination{
			Total:    total,
			PageSize: len(page),
			Offset:   offset,
			HasMore:  end < len(matched),
		},
	}, nil
}

func (s *MemoryStore) Watch(_ context.Context, in crudapi.WatchInput) (crudapi.WatchOutput, error) {
	p, err := crudapi.ParseURI(in.URI)
	if err != nil {
		return crudapi.WatchOutput{}, err
	}
	typ := p.Type()
	if typ == "" {
		return crudapi.WatchOutput{}, fmt.Errorf("URI %q has no type segment: %w", p.Path, crudapi.ErrInvalidInput)
	}
	// Memory backend supports type-wide watch only. ID granularity
	// requires the caller to filter locally on Event.ID; we still
	// return a subject covering the type so the subscription works.
	//
	// Tenant / universe scope is empty for the standalone MemoryStore
	// — ConnectionService is expected to wrap Watch and inject scope
	// from ctx at that layer once it exists. The subject is still
	// well-formed (the `ktw..universe...` prefix degrades to empty
	// tokens that are valid NATS path segments).
	subject := crudapi.EventSubject("", "", s.name, typ)
	return crudapi.WatchOutput{
		Subject: subject,
		EventTypes: []string{
			crudapi.WatchKindCreated,
			crudapi.WatchKindUpdated,
			crudapi.WatchKindDeleted,
		},
	}, nil
}

func (s *MemoryStore) Capabilities(_ context.Context) (crudapi.CapabilitiesOutput, error) {
	return crudapi.CapabilitiesOutput{
		Types: nil, // open — every Save creates a new type bucket on demand
		URIPatterns: []string{
			"{type}",      // collection (List / Watch) or new-record-with-server-id (Save)
			"{type}/{id}", // specific record (Load / Save / Delete)
		},
		Filters: crudapi.FiltersCap{
			Ops:     []string{"eq", "neq", "gt", "gte", "lt", "lte", "in", "nin", "contains", "startswith", "endswith"},
			Combine: "and",
		},
		Pagination: crudapi.PaginationCap{
			Modes:        []string{"offset"},
			DefaultLimit: 100,
			ReportsTotal: true,
		},
		Watch: crudapi.WatchCap{
			Supported:         true,
			Granularities:     []string{"type"},
			ServerSideFilters: false,
		},
		Distinct:     true,
		Projection:   false, // ListInput.Fields ignored — body is opaque to the backend
		Writable:     true,
		IDAssignment: "hybrid", // caller-provided ID wins; empty → server-assigned "<type>-<seq>"
	}, nil
}

// ----------------------------------------------------------------------
// Event emission
// ----------------------------------------------------------------------

func (s *MemoryStore) emit(_ context.Context, kind, typeName, id string, rec, old *crudapi.Record) {
	if s.eventBus == nil {
		return
	}
	evt := crudapi.Event{
		Kind:      kind,
		Type:      typeName,
		ID:        id,
		Record:    rec,
		OldRecord: old,
		Stamp:     s.nowFn(),
	}
	// Per-type subject so type-narrow subscribers don't see events for
	// other types. A wildcard subscriber would use the matching NATS
	// pattern at subscribe time; the EventBus surface treats this as a
	// plain string key today.
	subject := crudapi.EventSubject("", "", s.name, typeName)
	_ = s.eventBus.Publish(subject, evt)
}

// ----------------------------------------------------------------------
// Filter matching
// ----------------------------------------------------------------------

func supportedOp(op string) bool {
	switch op {
	case "eq", "neq", "gt", "gte", "lt", "lte", "in", "nin", "contains", "startswith", "endswith":
		return true
	}
	return false
}

// matchFilters returns true when every (field, filter) clause matches
// the record. AND-combined per CapabilitiesOutput.Filters.Combine ==
// "and". A clause referencing a missing field doesn't error — it just
// fails to match (mirrors SQL semantics on NULL comparison).
func matchFilters(rec crudapi.Record, filters map[string]crudapi.FilterValue) (bool, error) {
	if len(filters) == 0 {
		return true, nil
	}
	body, err := decodeBody(rec.Body)
	if err != nil {
		return false, fmt.Errorf("record %s/%s: %w", rec.Type, rec.ID, err)
	}
	for field, fv := range filters {
		ok, err := matchOne(body, field, fv)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func matchOne(body map[string]any, field string, fv crudapi.FilterValue) (bool, error) {
	got, present := body[field]
	switch fv.Op {
	case "eq":
		want, err := decodeScalar(fv.Value)
		if err != nil {
			return false, err
		}
		return present && equal(got, want), nil
	case "neq":
		want, err := decodeScalar(fv.Value)
		if err != nil {
			return false, err
		}
		return !equal(got, want), nil
	case "gt", "gte", "lt", "lte":
		want, err := decodeScalar(fv.Value)
		if err != nil {
			return false, err
		}
		return compareScalars(got, want, fv.Op)
	case "in":
		arr, err := decodeArray(fv.Value)
		if err != nil {
			return false, err
		}
		if !present {
			return false, nil
		}
		for _, v := range arr {
			if equal(got, v) {
				return true, nil
			}
		}
		return false, nil
	case "nin":
		arr, err := decodeArray(fv.Value)
		if err != nil {
			return false, err
		}
		if !present {
			return true, nil
		}
		for _, v := range arr {
			if equal(got, v) {
				return false, nil
			}
		}
		return true, nil
	case "contains", "startswith", "endswith":
		gs, ok := got.(string)
		if !ok {
			return false, nil
		}
		want, err := decodeScalar(fv.Value)
		if err != nil {
			return false, err
		}
		ws, ok := want.(string)
		if !ok {
			return false, fmt.Errorf("op %q expects string operand: %w", fv.Op, crudapi.ErrInvalidInput)
		}
		switch fv.Op {
		case "contains":
			return strings.Contains(gs, ws), nil
		case "startswith":
			return strings.HasPrefix(gs, ws), nil
		case "endswith":
			return strings.HasSuffix(gs, ws), nil
		}
	}
	return false, fmt.Errorf("op %q: %w", fv.Op, crudapi.ErrUnsupported)
}

func decodeBody(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("body decode: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func decodeScalar(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("filter value decode: %w", err)
	}
	return v, nil
}

func decodeArray(raw json.RawMessage) ([]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("filter array decode (expected JSON array): %w", err)
	}
	return arr, nil
}

func equal(a, b any) bool {
	// JSON-decoded numbers are float64 on both sides — direct == works
	// for scalars except heterogeneous int / float, which JSON unifies
	// for us. Slices / maps are compared via JSON round-trip only when
	// the caller passes them as filter values, which is rare; skip
	// here for simplicity.
	return a == b
}

func compareScalars(got, want any, op string) (bool, error) {
	gf, gok := asFloat(got)
	wf, wok := asFloat(want)
	if !gok || !wok {
		// Fall back to string lex compare for non-numerics.
		gs, gok := got.(string)
		ws, wok := want.(string)
		if !gok || !wok {
			return false, fmt.Errorf("op %q: operands not comparable (%T vs %T): %w", op, got, want, crudapi.ErrInvalidInput)
		}
		switch op {
		case "gt":
			return gs > ws, nil
		case "gte":
			return gs >= ws, nil
		case "lt":
			return gs < ws, nil
		case "lte":
			return gs <= ws, nil
		}
	}
	switch op {
	case "gt":
		return gf > wf, nil
	case "gte":
		return gf >= wf, nil
	case "lt":
		return gf < wf, nil
	case "lte":
		return gf <= wf, nil
	}
	return false, nil
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// ----------------------------------------------------------------------
// Sorting
// ----------------------------------------------------------------------

func sortRecords(recs []crudapi.Record, keys []crudapi.OrderBy) error {
	// Pre-decode bodies once so the Less func is fast (decoding inside
	// Less is O(n log n × decode cost) — bad).
	type cached struct {
		rec  crudapi.Record
		body map[string]any
	}
	decoded := make([]cached, len(recs))
	for i, r := range recs {
		body, err := decodeBody(r.Body)
		if err != nil {
			return err
		}
		decoded[i] = cached{rec: r, body: body}
	}
	sort.SliceStable(decoded, func(i, j int) bool {
		for _, k := range keys {
			ai, _ := decoded[i].body[k.Field]
			aj, _ := decoded[j].body[k.Field]
			cmp := compareAny(ai, aj)
			if cmp == 0 {
				continue
			}
			if k.Desc {
				return cmp > 0
			}
			return cmp < 0
		}
		// All keys equal — fall back to ID for stability.
		return decoded[i].rec.ID < decoded[j].rec.ID
	})
	for i, c := range decoded {
		recs[i] = c.rec
	}
	return nil
}

// compareAny returns -1 / 0 / +1 for arbitrary JSON-decoded scalars.
// Strings compared lex, numbers numerically, mismatched types treated
// as equal (stable sort breaks the tie via ID fallback above).
func compareAny(a, b any) int {
	if af, aok := asFloat(a); aok {
		if bf, bok := asFloat(b); bok {
			switch {
			case af < bf:
				return -1
			case af > bf:
				return 1
			default:
				return 0
			}
		}
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		switch {
		case as < bs:
			return -1
		case as > bs:
			return 1
		default:
			return 0
		}
	}
	return 0
}

// ----------------------------------------------------------------------
// Distinct
// ----------------------------------------------------------------------

// dedupRecords removes records whose Body matches a prior record byte-
// for-byte. Body is the only field worth deduping — Type / ID are
// already unique per record by definition.
func dedupRecords(recs []crudapi.Record) []crudapi.Record {
	seen := make(map[string]struct{}, len(recs))
	out := recs[:0]
	for _, r := range recs {
		key := string(r.Body)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r)
	}
	return out
}

// Compile-time assertion that MemoryStore satisfies the contract.
var _ crudapi.CRUDService = (*MemoryStore)(nil)
