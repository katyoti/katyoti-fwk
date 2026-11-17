// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// ErrUnknownPanel is a panel the server keeps nothing of.
var ErrUnknownPanel = errors.New("panel server: unknown panel")

// unknownPanel refuses a read of a panel the server keeps nothing of: the
// ErrUnknownPanel sentinel with the message key panelapi.MsgUnknownPanel, so a
// screen across the wire tells it by its key, never by its text.
func unknownPanel(id string) error {
	return usermsg.Wrap(fmt.Errorf("%w: %s", ErrUnknownPanel, id),
		panelapi.MsgUnknownPanel, map[string]any{"panel": id})
}

// Options configure a panel server. Only Dir is required.
type Options struct {
	// Dir is the server's folder: <DataDir>/panels.
	Dir string
	// Retention is how long an ended panel of a type is kept; 0 (or no
	// function) keeps it panelapi.DefaultRetention.
	Retention func(ctx context.Context, typeURI string) time.Duration
	// KeepOf is what a panel of a type keeps once it ends
	// (panel.TypeDecl.Keep): none, outcome or full; empty (or no function)
	// keeps nothing but what the person asks to keep.
	KeepOf func(ctx context.Context, typeURI string) string
	// Keep writes a kept panel's outcome as a document attached to object
	// and answers the document's path in the object's module. No function:
	// no outcome is kept.
	Keep func(ctx context.Context, object string, o panelapi.Outcome) (string, error)
	// TemplateBody reads a page template's body (a page-template
	// contribution's data) by its URI. No function: no template is known.
	TemplateBody func(ctx context.Context, uri string) ([]byte, error)
	// Coalesce is the least time between two deliveries to one follower:
	// what it takes meanwhile goes in the next one (default 100ms: at most
	// ten a second).
	Coalesce time.Duration
	// SnapshotEvery is the least time between two writes of a panel's
	// snapshot for its ops and progress (default 1s); an open, an ask, an
	// end and an input write it at once, and the log is always written
	// first, so a snapshot behind it is caught up when the panel is read.
	SnapshotEvery time.Duration
	// PruneEvery is how often Start prunes the expired panels (default 1h).
	PruneEvery time.Duration
	// Now is the clock (default time.Now).
	Now func() time.Time
}

// Server is the panel server: the one writer of its folder.
type Server struct {
	st   store
	opts Options

	mu     sync.Mutex
	panels map[string]*entry
	ctx    context.Context // what the keeps run in: Start's, else Background

	keeps sync.WaitGroup // the outcomes being written
}

// entry is one panel in memory: its snapshot, the order its events are taken
// in, the screens following it.
type entry struct {
	mu        sync.Mutex
	snap      panelapi.Snapshot
	known     bool
	order     *eventsapi.PanelFollower[panel.Event]
	ahead     map[int64]struct{} // the sequences held, waiting on one before them
	followers map[*follower]struct{}
	written   time.Time
	dirty     bool
	keeping   bool // its outcome is being written
}

// New is a panel server over opts.Dir.
func New(opts Options) (*Server, error) {
	if opts.Dir == "" {
		return nil, fmt.Errorf("panel server: no folder")
	}
	if opts.Coalesce <= 0 {
		opts.Coalesce = 100 * time.Millisecond
	}
	if opts.SnapshotEvery <= 0 {
		opts.SnapshotEvery = time.Second
	}
	if opts.PruneEvery <= 0 {
		opts.PruneEvery = time.Hour
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Server{st: store{dir: opts.Dir}, opts: opts, panels: map[string]*entry{}, ctx: context.Background()}, nil
}

func (s *Server) baseCtx() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ctx
}

// panel is the entry of id, read from the folder the first time.
func (s *Server) panel(id string) (*entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.panels[id]; ok {
		return e, nil
	}
	snap, known, err := s.st.load(id)
	if err != nil {
		return nil, err
	}
	if !known {
		snap = panelapi.Snapshot{Panel: id}
	}
	e := &entry{snap: snap, known: known, followers: map[*follower]struct{}{}, ahead: map[int64]struct{}{}}
	e.order = eventsapi.NewPanelFollower(snap.Seq, panel.SeqOf)
	e.written = s.opts.Now()
	s.panels[id] = e
	return e, nil
}

// Take takes one event out of a panel: in its sequence order - one that
// arrives early waits for those before it, one already taken is dropped -,
// each appended to the log, folded into the snapshot and handed to the
// screens following the panel. A gap no later event fills is passed over
// once more than eventsapi.PanelReorderWindow events wait on it.
func (s *Server) Take(e panel.Event) error {
	if err := e.Validate(); err != nil {
		return err
	}
	en, err := s.panel(e.Panel)
	if err != nil {
		return err
	}
	en.mu.Lock()
	defer en.mu.Unlock()
	if e.Seq > en.order.Seq() {
		en.ahead[e.Seq] = struct{}{}
	}
	ready, resync := en.order.Offer(e)
	if resync {
		// The server is the record: no snapshot to read again. It passes
		// over the gap, to the first event held.
		first := int64(0)
		for seq := range en.ahead {
			if seq > en.order.Seq() && (first == 0 || seq < first) {
				first = seq
			}
		}
		slog.Warn("panel server: events lost on the way - passed over", "panel", e.Panel, "from", en.order.Seq()+1, "to", first-1)
		ready = append(ready, en.order.Reset(first-1)...)
	}
	for seq := range en.ahead {
		if seq <= en.order.Seq() {
			delete(en.ahead, seq)
		}
	}
	var errs []error
	for _, ev := range ready {
		now := s.opts.Now()
		if err := s.st.appendRecord(ev.Panel, panelapi.Record{At: now, Event: &ev}); err != nil {
			errs = append(errs, err)
			continue
		}
		fold(&en.snap, ev, now)
		en.known = true
		at := ev.Kind != panel.KindOps && ev.Kind != panel.KindProgress
		if err := s.saveLocked(en, at); err != nil {
			errs = append(errs, err)
		}
	}
	if len(ready) > 0 {
		for f := range en.followers {
			f.push(ready)
		}
		s.keepLocked(en)
	}
	return errors.Join(errs...)
}

// Input records one input the person sent a panel: appended to its log
// after the last event taken. The server does not route it - the owning
// run waits on it itself.
func (s *Server) Input(in panel.Input) error {
	if err := in.Validate(); err != nil {
		return err
	}
	en, err := s.panel(in.Panel)
	if err != nil {
		return err
	}
	en.mu.Lock()
	defer en.mu.Unlock()
	now := s.opts.Now()
	if err := s.st.appendRecord(in.Panel, panelapi.Record{At: now, Input: &in, After: en.snap.Seq}); err != nil {
		return err
	}
	take(&en.snap, now)
	en.known = true
	return s.saveLocked(en, true)
}

// saveLocked writes the entry's snapshot now, or once SnapshotEvery has
// passed since the last write; else it is left for Flush.
func (s *Server) saveLocked(en *entry, now bool) error {
	t := s.opts.Now()
	if !now && t.Sub(en.written) < s.opts.SnapshotEvery {
		en.dirty = true
		return nil
	}
	if err := s.st.writeSnapshot(en.snap); err != nil {
		en.dirty = true
		return err
	}
	en.written, en.dirty = t, false
	return nil
}

// Flush writes every snapshot left behind its log.
func (s *Server) Flush() error {
	s.mu.Lock()
	entries := make([]*entry, 0, len(s.panels))
	for _, e := range s.panels {
		entries = append(entries, e)
	}
	s.mu.Unlock()
	var errs []error
	for _, en := range entries {
		en.mu.Lock()
		if en.dirty {
			errs = append(errs, s.saveLocked(en, true))
		}
		en.mu.Unlock()
	}
	return errors.Join(errs...)
}

// Snapshot answers the panel after its last event.
func (s *Server) Snapshot(_ context.Context, in panelapi.SnapshotInput) (panelapi.Snapshot, error) {
	if !panel.ValidID(in.Panel) {
		return panelapi.Snapshot{}, fmt.Errorf("panel server: %q is no panel id", in.Panel)
	}
	en, err := s.panel(in.Panel)
	if err != nil {
		return panelapi.Snapshot{}, err
	}
	en.mu.Lock()
	defer en.mu.Unlock()
	if !en.known {
		return panelapi.Snapshot{}, unknownPanel(in.Panel)
	}
	return copySnapshot(en.snap), nil
}

// History answers the panel's log after From: the events after it and the
// inputs taken after it, in the order taken.
func (s *Server) History(_ context.Context, in panelapi.HistoryInput) (panelapi.HistoryOutput, error) {
	if !panel.ValidID(in.Panel) {
		return panelapi.HistoryOutput{}, fmt.Errorf("panel server: %q is no panel id", in.Panel)
	}
	en, err := s.panel(in.Panel)
	if err != nil {
		return panelapi.HistoryOutput{}, err
	}
	en.mu.Lock()
	defer en.mu.Unlock()
	if !en.known {
		return panelapi.HistoryOutput{}, unknownPanel(in.Panel)
	}
	records, err := s.st.readLog(in.Panel)
	if err != nil {
		return panelapi.HistoryOutput{}, err
	}
	out := panelapi.HistoryOutput{Records: []panelapi.Record{}}
	for _, r := range records {
		if (r.Event != nil && r.Event.Seq > in.From) || (r.Input != nil && r.After >= in.From) {
			out.Records = append(out.Records, r)
		}
	}
	return out, nil
}

// List answers every panel kept, the last changed first.
func (s *Server) List(_ context.Context, in panelapi.ListInput) (panelapi.ListOutput, error) {
	ids, err := s.st.ids()
	if err != nil {
		return panelapi.ListOutput{}, err
	}
	out := panelapi.ListOutput{Panels: []panelapi.Summary{}}
	for _, id := range ids {
		en, err := s.panel(id)
		if err != nil {
			return panelapi.ListOutput{}, err
		}
		en.mu.Lock()
		sn, known := en.snap, en.known
		en.mu.Unlock()
		if !known || (in.Owner != "" && sn.Owner != in.Owner) {
			continue
		}
		out.Panels = append(out.Panels, panelapi.Summary{Panel: sn.Panel, Seq: sn.Seq, State: sn.State,
			Type: sn.Type, Title: sn.Title, Owner: sn.Owner, Waiting: sn.Waiting, Updated: sn.Updated})
	}
	sort.SliceStable(out.Panels, func(i, j int) bool { return out.Panels[i].Updated.After(out.Panels[j].Updated) })
	return out, nil
}

// Prune deletes the panels whose retention passed at now - their type's, as
// ended runs by default - and answers their ids. A panel waiting for an
// answer is kept, and so is one whose outcome is being written. A panel
// whose outcome is due and not written yet - its keeper failed - is tried
// again while its retention runs.
func (s *Server) Prune(ctx context.Context, now time.Time) ([]string, error) {
	ids, err := s.st.ids()
	if err != nil {
		return nil, err
	}
	var pruned []string
	var errs []error
	for _, id := range ids {
		en, err := s.panel(id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		en.mu.Lock()
		if en.keeping {
			en.mu.Unlock()
			continue
		}
		if !expired(en.snap, s.retention(ctx, en.snap.Type), now) {
			s.keepLocked(en)
		} else {
			if err := s.st.remove(id); err != nil {
				errs = append(errs, err)
			} else {
				pruned = append(pruned, id)
				s.mu.Lock()
				delete(s.panels, id)
				s.mu.Unlock()
			}
		}
		en.mu.Unlock()
	}
	return pruned, errors.Join(errs...)
}

func (s *Server) retention(ctx context.Context, typeURI string) time.Duration {
	if s.opts.Retention != nil {
		if d := s.opts.Retention(ctx, typeURI); d > 0 {
			return d
		}
	}
	return panelapi.DefaultRetention
}

// Follow delivers the panel's events after from, then each one as it is
// taken, coalesced: at most one delivery per Coalesce, with every event taken
// since the last. Deliveries are in order and never inside the server's
// locks. stop ends the follow.
func (s *Server) Follow(id string, from int64, deliver func([]panel.Event)) (func(), error) {
	if !panel.ValidID(id) {
		return nil, fmt.Errorf("panel server: %q is no panel id", id)
	}
	if deliver == nil {
		return nil, fmt.Errorf("panel server: follow %s: nothing to deliver to", id)
	}
	en, err := s.panel(id)
	if err != nil {
		return nil, err
	}
	en.mu.Lock()
	records, err := s.st.readLog(id)
	if err != nil {
		en.mu.Unlock()
		return nil, err
	}
	f := &follower{every: s.opts.Coalesce, now: s.opts.Now, deliver: deliver}
	var past []panel.Event
	for _, r := range records {
		if r.Event != nil && r.Event.Seq > from {
			past = append(past, *r.Event)
		}
	}
	f.push(past)
	en.followers[f] = struct{}{}
	en.mu.Unlock()
	return func() {
		en.mu.Lock()
		delete(en.followers, f)
		en.mu.Unlock()
		f.stop()
	}, nil
}

// Start subscribes the server to every panel subject of the universe -
// events out and inputs in - and prunes the expired panels now and every
// PruneEvery, writing the snapshots left behind on the way. stop ends it.
func (s *Server) Start(ctx context.Context, bus mbapi.MessageBus, tenant, universe string) (func(), error) {
	s.mu.Lock()
	s.ctx = context.WithoutCancel(ctx)
	s.mu.Unlock()
	sub, err := bus.Subscribe(ctx, subjects.PanelsPattern(tenant, universe), func(_ context.Context, msg mbapi.Message) {
		s.handle(tenant, universe, msg.Subject(), msg.Data())
	})
	if err != nil {
		return nil, fmt.Errorf("panel server: subscribe: %w", err)
	}
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.pruneLogged(loopCtx)
		flush := time.NewTicker(s.opts.SnapshotEvery)
		prune := time.NewTicker(s.opts.PruneEvery)
		defer flush.Stop()
		defer prune.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-flush.C:
				if err := s.Flush(); err != nil {
					slog.Warn("panel server: snapshot not written", "err", err)
				}
			case <-prune.C:
				s.pruneLogged(loopCtx)
			}
		}
	}()
	return func() {
		_ = sub.Unsubscribe()
		cancel()
		<-done
		s.WaitKeeps()
		if err := s.Flush(); err != nil {
			slog.Warn("panel server: snapshot not written", "err", err)
		}
	}, nil
}

func (s *Server) pruneLogged(ctx context.Context) {
	pruned, err := s.Prune(ctx, s.opts.Now())
	if err != nil {
		slog.Warn("panel server: prune", "err", err)
	}
	if len(pruned) > 0 {
		slog.Info("panel server: ended panels pruned", "count", len(pruned))
	}
}

// handle takes one message off the panel subjects: an event out or an input
// in, told apart by the subject builders.
func (s *Server) handle(tenant, universe, subject string, data []byte) {
	var probe struct {
		Panel string `json:"panel"`
	}
	if json.Unmarshal(data, &probe) != nil || !panel.ValidID(probe.Panel) {
		slog.Warn("panel server: a message names no panel", "subject", subject)
		return
	}
	switch subject {
	case subjects.PanelOutSubject(tenant, universe, probe.Panel):
		e, err := panel.DecodeEvent(data)
		if err == nil {
			err = s.Take(e)
		}
		if err != nil {
			slog.Warn("panel server: event not kept", "panel", probe.Panel, "err", err)
		}
	case subjects.PanelInSubject(tenant, universe, probe.Panel):
		in, err := panel.DecodeInput(data)
		if err == nil {
			err = s.Input(in)
		}
		if err != nil {
			slog.Warn("panel server: input not kept", "panel", probe.Panel, "err", err)
		}
	default:
		slog.Warn("panel server: a message off its panel's subject", "subject", subject, "panel", probe.Panel)
	}
}

func copySnapshot(s panelapi.Snapshot) panelapi.Snapshot {
	s.Events = append([]panel.Event(nil), s.Events...)
	return s
}

// follower is one screen following a panel: what it has not been handed yet
// and when it was last handed something.
type follower struct {
	every   time.Duration
	now     func() time.Time
	deliver func([]panel.Event)

	mu      sync.Mutex
	pending []panel.Event
	last    time.Time
	timer   *time.Timer
	stopped bool

	out sync.Mutex // one delivery at a time, in order
}

// push queues events and plans their delivery: at once when the last one is
// Coalesce old, else when it will be.
func (f *follower) push(events []panel.Event) {
	if len(events) == 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return
	}
	f.pending = append(f.pending, events...)
	if f.timer != nil {
		return
	}
	wait := f.every - f.now().Sub(f.last)
	if wait < 0 {
		wait = 0
	}
	f.timer = time.AfterFunc(wait, f.flush)
}

func (f *follower) flush() {
	f.out.Lock()
	defer f.out.Unlock()
	f.mu.Lock()
	batch := f.pending
	f.pending, f.timer = nil, nil
	f.last = f.now()
	stopped := f.stopped
	f.mu.Unlock()
	if stopped || len(batch) == 0 {
		return
	}
	f.deliver(batch)
}

func (f *follower) stop() {
	f.mu.Lock()
	f.stopped = true
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
	f.pending = nil
	f.mu.Unlock()
}
