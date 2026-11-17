// SPDX-License-Identifier: Apache-2.0

// Package core implements the journal over the CWM Object API: the recorder
// the CWM event observer calls, and the queries.
package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
)

// JournalServiceConfig is JournalService.yaml.
type JournalServiceConfig struct {
	// RetentionDays is how many days stay in the tree; older journal-day
	// objects are pruned on the first record of a day (git keeps them).
	RetentionDays int `yaml:"retention_days,omitempty" json:"retention_days,omitempty" default:"90"`
}

// Service is the journal. One per process; only the process holding the CWM
// workspace records (the observer is attached there).
type Service struct {
	retention int
	now       func() time.Time

	mu       sync.Mutex
	days     map[string]string // project key ("" = one project) → the journal-day known to exist
	runtimes map[string]bool   // type → runtime, as the catalog declares it
}

// New builds the journal; retentionDays <= 0 means the default.
func New(retentionDays int) *Service {
	if retentionDays <= 0 {
		retentionDays = api.DefaultRetentionDays
	}
	return &Service{retention: retentionDays, now: time.Now, days: map[string]string{}, runtimes: map[string]bool{}}
}

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	return application.UseService[cwmapi.CWMService](ctx)
}

// Record turns one CWM event into at most one journal line. It is the observer
// the journal extension contributes; events are handed over in order.
//
// The record goes into the journal of the project whose store wrote the event
// (cwmapi.EventProject): with several projects served, each project keeps its
// own journal, <key>:sys-journal.
//
// Not recorded: the writes an operation's own event reports (a moved object's
// own, a split source's removal), the journal's own module (it would record
// itself), and objects of a runtime type (uncommitted state has no committed
// history).
func (s *Service) Record(ctx context.Context, ev cwmapi.EventEnvelope) error {
	rec, ok := recordOf(ev)
	if !ok || rec.Module == api.Module || rec.ID == "" {
		return nil
	}
	if rec.At.IsZero() {
		rec.At = s.now()
	}
	rec.At = rec.At.UTC()
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	if s.isRuntime(ctx, cwm, rec.Type) {
		return nil
	}
	return s.write(ctx, cwm, cwmapi.EventProject(ctx), rec)
}

// Invoked records that an action ran on an object, in the journal of the
// project holding it: the id qualified <key>:<module>.<local>, or a bare id
// held by one project.
func (s *Service) Invoked(ctx context.Context, in api.InvokedInput) (api.InvokedOutput, error) {
	if in.ID == "" || in.URI == "" || in.Status == "" {
		return api.InvokedOutput{}, fmt.Errorf("%w: journal.Invoked: id, uri and status are required", cwmapi.ErrInvalidInput)
	}
	key, id, err := cwmapi.SplitReference(in.ID)
	if err != nil {
		return api.InvokedOutput{}, err
	}
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return api.InvokedOutput{}, err
	}
	note := cwmapi.CutNote(in.Note) // what came of the action, bounded
	rec := api.Record{At: s.now().UTC(), Action: api.ActionInvoked, ID: id, Module: module,
		URI: in.URI, Status: in.Status, Note: note, By: in.By}
	if c, ok := cause.CauseFrom(ctx); ok {
		rec.Cause = &c
	}
	if module == api.Module {
		return api.InvokedOutput{Record: rec}, nil
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.InvokedOutput{}, err
	}
	if key == "" {
		key = holderOf(ctx, cwm, id)
	}
	return api.InvokedOutput{Record: rec}, s.write(ctx, cwm, key, rec)
}

// holderOf is the key of the project holding the bare id: "" when one project
// is served, or when no project or several hold it.
func holderOf(ctx context.Context, cwm cwmapi.CWMService, id string) string {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return ""
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return ""
	}
	return got.Project
}

// scopeOf reads an object reference for the queries: the project whose
// journal holds its records, and its bare id. A qualified id names its
// project; a bare id held by one project reads that project's journal; a bare
// id held by none (a deleted object) or by several reads every project's
// journal (key "").
func scopeOf(ctx context.Context, cwm cwmapi.CWMService, ref string) (key, id string) {
	key, id, err := cwmapi.SplitReference(ref)
	if err != nil {
		return "", ref
	}
	if key == "" {
		key = holderOf(ctx, cwm, id)
	}
	return key, id
}

// qualify writes id in the project of key: bare when key is "".
func qualify(key, id string) string {
	if key == "" {
		return id
	}
	return cwmapi.QualifiedID(key, id)
}

// write appends one record to its day, in the journal of the project of key
// ("" = the one project served).
func (s *Service) write(ctx context.Context, cwm cwmapi.CWMService, key string, rec api.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	day, err := s.ensureDay(ctx, cwm, key, rec.At)
	if err != nil {
		return err
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = cwm.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{
		ID: day, Name: api.EntriesFile, Content: append(line, '\n'), Append: true,
	})
	return err
}

// recordOf reads an event envelope. The topic ends `<entity>.<token>.<action>`.
// The event's meta — its id, the write's time, the revision, the cause — is
// the record's; the envelope's timestamp (when the observer received it)
// stands in only for an event that carries no time.
func recordOf(ev cwmapi.EventEnvelope) (api.Record, bool) {
	segs := strings.Split(ev.Topic, ".")
	if len(segs) < 3 {
		return api.Record{}, false
	}
	entity, action := segs[len(segs)-3], segs[len(segs)-1]
	rec := api.Record{At: ev.Timestamp, Action: action}
	meta := func(m cwmapi.EventMeta) {
		rec.EventID, rec.Revision, rec.Cause = m.EventID, m.Revision, m.Cause
		if !m.At.IsZero() {
			rec.At = m.At
		}
	}
	switch {
	case entity == "object" && (action == api.ActionCreated || action == api.ActionUpdated || action == api.ActionDeleted):
		var p cwmapi.ObjectEventPayload
		// A write whose operation publishes its own event (a move, a split) is
		// reported by that event. A transition's own write is recorded as it
		// always was, beside the transitioned record.
		if json.Unmarshal(ev.Payload, &p) != nil || p.ObjectID == "" ||
			p.Operation == cwmapi.OperationMove || p.Operation == cwmapi.OperationSplit {
			return api.Record{}, false
		}
		meta(p.EventMeta)
		rec.ID, rec.Module, rec.Type, rec.Parent = p.ObjectID, p.Module, p.Type, p.Parent
	case entity == "object" && action == api.ActionMoved:
		var p cwmapi.ObjectMovedEventPayload
		if json.Unmarshal(ev.Payload, &p) != nil {
			return api.Record{}, false
		}
		meta(p.EventMeta)
		rec.ID, rec.Module, rec.Type, rec.Parent = p.ObjectID, p.Module, p.Type, p.Parent
		rec.FromID, rec.FromModule, rec.FromParent, rec.Reason = p.FromID, p.FromModule, p.FromParent, p.Reason
	case entity == "object" && action == api.ActionSplit:
		var p cwmapi.ObjectSplitEventPayload
		if json.Unmarshal(ev.Payload, &p) != nil {
			return api.Record{}, false
		}
		kept := p.SourceKept
		meta(p.EventMeta)
		rec.ID, rec.Module, rec.Type, rec.Parent = p.ObjectID, p.Module, p.Type, p.Parent
		rec.Successors, rec.SourceKept, rec.Reason = p.Successors, &kept, p.Reason
	case entity == "object" && action == api.ActionArchived:
		// The object ended: its file left for its module's history folder.
		var p cwmapi.ObjectArchivedEventPayload
		if json.Unmarshal(ev.Payload, &p) != nil || p.ObjectID == "" {
			return api.Record{}, false
		}
		meta(p.EventMeta)
		rec.ID, rec.Module, rec.Type, rec.Parent, rec.Note = p.ObjectID, p.Module, p.Type, p.Parent, p.Path
	case entity == "object" && action == api.ActionEvent:
		// An event raised on the object (EventRaise): its type, its payload cut.
		var p cwmapi.ObjectRaisedEventPayload
		if json.Unmarshal(ev.Payload, &p) != nil || p.ObjectID == "" {
			return api.Record{}, false
		}
		meta(p.EventMeta)
		rec.ID, rec.Module, rec.Type = p.ObjectID, p.Module, p.Type
		rec.Event, rec.Note = p.Event, p.Summary
	case entity == "object" && action == api.ActionTransitioned:
		var p cwmapi.ObjectTransitionedEventPayload
		if json.Unmarshal(ev.Payload, &p) != nil {
			return api.Record{}, false
		}
		by := p.By
		meta(p.EventMeta)
		rec.ID, rec.Module, rec.Type = p.ObjectID, p.Module, p.Type
		rec.FromState, rec.ToState, rec.Transition, rec.By, rec.Note = p.From, p.To, p.Transition, &by, p.Note
		rec.Capability, rec.Forced, rec.Calls = p.Capability, p.Forced, p.Calls
	default:
		return api.Record{}, false
	}
	return rec, true
}

func (s *Service) isRuntime(ctx context.Context, cwm cwmapi.CWMService, typ string) bool {
	if typ == "" {
		return false
	}
	s.mu.Lock()
	rt, known := s.runtimes[typ]
	s.mu.Unlock()
	if known {
		return rt
	}
	out, err := cwm.ObjectTypeGet(ctx, cwmapi.ObjectTypeGetInput{Name: typ})
	rt = err == nil && out.ObjectType.Runtime
	s.mu.Lock()
	s.runtimes[typ] = rt
	s.mu.Unlock()
	return rt
}

// ensureDay makes the journal-day of at exist in the project of key — and its
// sys-journal with it — and, the first time a day is written there, prunes
// that project's days past the retention. It answers the day's id, qualified
// when key is named.
func (s *Service) ensureDay(ctx context.Context, cwm cwmapi.CWMService, key string, at time.Time) (string, error) {
	bare := api.DayID(at)
	id := qualify(key, bare)
	if s.days[key] == id {
		return id, nil
	}
	if err := ensureModule(ctx, cwm, key); err != nil {
		return "", err
	}
	day := cwmapi.Object{ID: bare, Type: api.DayType}
	day.Name = at.Format("2006-01-02")
	day.Parent = api.Module
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: qualify(key, api.Module), Object: day}); err != nil && !errors.Is(err, cwmapi.ErrAlreadyExists) {
		return "", fmt.Errorf("journal: create %s: %w", id, err)
	}
	s.days[key] = id
	if err := s.prune(ctx, cwm, key, at); err != nil {
		return id, fmt.Errorf("journal: prune: %w", err)
	}
	return id, nil
}

// ensureModule creates the project's sys-journal on the journal's first write
// there: the journal has no bootstrap hook — a project that never records
// anything has no sys-journal.
func ensureModule(ctx context.Context, cwm cwmapi.CWMService, key string) error {
	_, err := cwmapi.EnsureSystemModule(ctx, cwm, qualify(key, api.Module),
		"The journal's operational space: one journal-day per day, what happened to every object (committed).")
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	return nil
}

// prune deletes the project's journal-days older than the retention, with
// their entries.
func (s *Service) prune(ctx context.Context, cwm cwmapi.CWMService, key string, now time.Time) error {
	cutoff := now.UTC().AddDate(0, 0, -s.retention).Format("2006-01-02")
	days, err := listDays(ctx, cwm, key)
	if err != nil {
		return err
	}
	for _, d := range days {
		if d.date >= cutoff {
			continue
		}
		read, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: d.id, Name: api.EntriesFile, Limit: 1})
		if err == nil && read.File.Exists {
			if _, err := cwm.ObjectFileDelete(ctx, cwmapi.ObjectFileDeleteInput{ID: d.id, Path: read.File.Path}); err != nil {
				return err
			}
		}
		if _, err := cwm.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: api.Module, ID: d.id}); err != nil {
			return err
		}
	}
	return nil
}

// dayRef is one journal-day: its id - qualified by its project when several
// projects are served - and its date.
type dayRef struct{ id, date string }

// listDays is every journal-day of the project of key, newest first; key ""
// is every project's (the one project's when one is served).
func listDays(ctx context.Context, cwm cwmapi.CWMService, key string) ([]dayRef, error) {
	out, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: api.Module, Type: api.DayType})
	if err != nil {
		return nil, err
	}
	days := make([]dayRef, 0, len(out.Objects))
	for i, o := range out.Objects {
		project := ""
		if i < len(out.Projects) {
			project = out.Projects[i]
		}
		if key != "" && project != key {
			continue
		}
		days = append(days, dayRef{id: qualify(project, o.ID), date: o.Name})
	}
	sort.Slice(days, func(i, j int) bool { return days[i].date > days[j].date })
	return days, nil
}

// Query reads the days the window covers, newest first, and filters. An
// object named reads the journal of the project holding it (scopeOf); a
// module written <key>:<module> reads that project's; else every project's.
func (s *Service) Query(ctx context.Context, in api.QueryInput) (api.QueryOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.QueryOutput{}, err
	}
	key := ""
	if i := strings.Index(in.Module, ":"); i >= 0 {
		key, in.Module = in.Module[:i], in.Module[i+1:]
	}
	id := in.ObjectID
	if id != "" {
		var held string
		held, id = scopeOf(ctx, cwm, id)
		if key != "" && held != "" && held != key {
			return api.QueryOutput{Records: []api.Record{}}, nil // the object is not in the module's project
		}
		if held != "" {
			key = held
		}
	}
	return s.query(ctx, key, in, func(r api.Record) bool {
		return id == "" || r.ID == id || r.FromID == id
	})
}

// History is everything that happened to one object, newest first, followed
// back through its moves and splits: a move names the id it came from, and a
// split names its successors, so older records under the source are the
// successor's history too.
func (s *Service) History(ctx context.Context, in api.HistoryInput) (api.QueryOutput, error) {
	if in.ID == "" {
		return api.QueryOutput{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.QueryOutput{}, err
	}
	key, id := scopeOf(ctx, cwm, in.ID)
	return s.query(ctx, key, api.QueryInput{Limit: in.Limit}, followed(id))
}

// followed matches the records about id, followed back through its moves and
// splits — records are read newest first, so a move met names the older id.
func followed(id string) func(api.Record) bool {
	ids := map[string]bool{id: true}
	return func(r api.Record) bool {
		if r.Action == api.ActionSplit && !ids[r.ID] {
			for _, succ := range r.Successors {
				if ids[succ] {
					ids[r.ID] = true
					break
				}
			}
		}
		if !ids[r.ID] {
			return false
		}
		if r.Action == api.ActionMoved && r.FromID != "" {
			ids[r.FromID] = true
		}
		return true
	}
}

// query reads the journal of the project of key ("" = every project's),
// newest first.
func (s *Service) query(ctx context.Context, key string, in api.QueryInput, match func(api.Record) bool) (api.QueryOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.QueryOutput{}, err
	}
	limit := in.Limit
	if limit <= 0 {
		limit = api.DefaultLimit
	}
	actions := map[string]bool{}
	for _, a := range in.Actions {
		actions[a] = true
	}
	out := api.QueryOutput{Records: []api.Record{}}
	days, err := listDays(ctx, cwm, key)
	if err != nil {
		return out, err
	}
	for _, d := range days {
		if !in.Until.IsZero() && d.date > in.Until.UTC().Format("2006-01-02") {
			continue
		}
		if !in.Since.IsZero() && d.date < in.Since.UTC().Format("2006-01-02") {
			break // days are newest first: every later one is older still
		}
		read, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: d.id, Name: api.EntriesFile})
		if err != nil {
			return out, err
		}
		recs := parseLines(read.Content)
		for i := len(recs) - 1; i >= 0; i-- { // appended in order: the last line is the newest
			r := recs[i]
			if !in.Since.IsZero() && r.At.Before(in.Since) || !in.Until.IsZero() && !r.At.Before(in.Until) {
				continue
			}
			if in.Module != "" && r.Module != in.Module && r.FromModule != in.Module {
				continue
			}
			if len(actions) > 0 && !actions[r.Action] {
				continue
			}
			if !match(r) {
				continue
			}
			if len(out.Records) == limit {
				out.Truncated = true
				return out, nil
			}
			out.Records = append(out.Records, r)
		}
	}
	return out, nil
}

// parseLines reads a JSONL file; a line that does not decode is skipped (a
// torn last line after a crash must not hide the day).
func parseLines(b []byte) []api.Record {
	var out []api.Record
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r api.Record
		if json.Unmarshal(line, &r) == nil {
			out = append(out, r)
		}
	}
	return out
}
