// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
)

// clock is a test clock the server reads.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newServer(t *testing.T, dir string, c *clock, retention func(context.Context, string) time.Duration) *Server {
	t.Helper()
	s, err := New(Options{Dir: dir, Now: c.now, Coalesce: 20 * time.Millisecond, Retention: retention})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// events drives one panel through a track, as a run's steps would.
type events struct {
	t  *testing.T
	tr panel.Track
}

func (p *events) next(e panel.Event) panel.Event {
	p.t.Helper()
	out, err := p.tr.Next(e)
	if err != nil {
		p.t.Fatal(err)
	}
	return out
}

func tree() *render.Spec {
	return &render.Spec{Inline: render.Node{"kind": "vbox", "id": "root", "children": []any{
		map[string]any{"kind": "text", "id": "status", "content": "starting"},
	}}}
}

func openEvent(p *events, typ string) panel.Event {
	return p.next(panel.Event{Kind: panel.KindOpen, Open: &panel.Open{Type: typ, Title: "Publication", Owner: "run-1", Answer: render.Answer{Tree: tree()}}})
}

func text(s string) render.Node { return render.Node{"kind": "text", "content": s} }

func mustTake(t *testing.T, s *Server, e panel.Event) {
	t.Helper()
	if err := s.Take(e); err != nil {
		t.Fatal(err)
	}
}

// A panel server keeps each panel's log and snapshot in the universe's data
// folder: each event appended to the log, the snapshot folded
// and compacted, a screen following from a sequence, the panel read again
// after a restart, the ended panels pruned after their type's retention and a
// panel waiting for an answer kept, the bus's events and inputs taken in
// their order, and the snapshot and the history read over HTTP.
func TestAPanelServerKeepsEachPanelsLogAndSnapshot(t *testing.T) {
	ctx := context.Background()

	t.Run("append and snapshot", func(t *testing.T) {
		dir := t.TempDir()
		c := &clock{t: time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)}
		s := newServer(t, dir, c, nil)
		p := &events{t: t, tr: panel.NewTrack("")}
		id := p.tr.ID
		mustTake(t, s, openEvent(p, "ktw-agents:/panel-type/steps"))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindOps, Ops: []render.Op{{Op: render.OpReplace, Target: "status", Node: text("one")}}}))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Label: "pages", Current: 1, Total: 3}}))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindOps, Ops: []render.Op{{Op: render.OpReplace, Target: "status", Node: text("two")}}}))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Label: "pages", Current: 2, Total: 3}}))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindAsk, Ask: &panel.Ask{Question: "Publish?"}}))

		records, err := s.st.readLog(id)
		if err != nil || len(records) != 6 {
			t.Fatalf("the log holds %d lines (%v), want each of the 6 events", len(records), err)
		}
		if _, err := os.Stat(filepath.Join(dir, id+".jsonl")); err != nil {
			t.Fatalf("the log is not <dir>/<id>.jsonl: %v", err)
		}
		snap, err := s.Snapshot(ctx, panelapi.SnapshotInput{Panel: id})
		if err != nil {
			t.Fatal(err)
		}
		if snap.Seq != 6 || snap.State != panel.StateAsking || !snap.Waiting || snap.Title != "Publication" || snap.Owner != "run-1" {
			t.Fatalf("snapshot = seq %d state %s waiting %v %q %q, want seq 6, asking, waiting, its title and owner", snap.Seq, snap.State, snap.Waiting, snap.Title, snap.Owner)
		}
		// Compacted: the open, the last replace of status, the last
		// progress, the ask - in their order, each with its own seq.
		var got []string
		for _, e := range snap.Events {
			got = append(got, string(e.Kind)+":"+itoa(e.Seq))
		}
		if strings.Join(got, " ") != "open:1 ops:4 progress:5 ask:6" {
			t.Fatalf("snapshot events = %v, want open:1 ops:4 progress:5 ask:6", got)
		}
		// An ops event after the ask: the panel runs again and the ask leaves.
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindOps, Ops: []render.Op{
			{Op: render.OpRemove, Target: "status"}, {Op: render.OpAppend, Target: "root", Items: []any{"x"}},
		}}))
		snap, _ = s.Snapshot(ctx, panelapi.SnapshotInput{Panel: id})
		got = nil
		for _, e := range snap.Events {
			got = append(got, string(e.Kind)+":"+itoa(e.Seq))
		}
		if snap.State != panel.StateOpen || snap.Waiting || strings.Join(got, " ") != "open:1 progress:5 ops:7" {
			t.Fatalf("after ops: state %s waiting %v events %v, want open, not waiting, open:1 progress:5 ops:7 (the replaced status superseded by its remove)", snap.State, snap.Waiting, got)
		}
		if err := s.Flush(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, id+".snapshot.json")); err != nil {
			t.Fatalf("the snapshot is not <dir>/<id>.snapshot.json: %v", err)
		}
	})

	t.Run("an event taken twice or early keeps the order", func(t *testing.T) {
		c := &clock{t: time.Now()}
		s := newServer(t, t.TempDir(), c, nil)
		p := &events{t: t, tr: panel.NewTrack("")}
		e1 := openEvent(p, "")
		e2 := p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Current: 1}})
		e3 := p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Current: 2}})
		for _, e := range []panel.Event{e1, e3, e2, e2, e1} {
			mustTake(t, s, e)
		}
		records, _ := s.st.readLog(p.tr.ID)
		var seqs []string
		for _, r := range records {
			seqs = append(seqs, itoa(r.Event.Seq))
		}
		if strings.Join(seqs, ",") != "1,2,3" {
			t.Fatalf("log order = %v, want 1,2,3 once each", seqs)
		}
		// An event lost on the way: the server is the record, it passes
		// over the gap once more than the reorder window waits on it.
		p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Current: 3}}) // seq 4, never taken
		for i := 0; i < 17; i++ {
			mustTake(t, s, p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Current: int64(5 + i)}}))
		}
		snap, _ := s.Snapshot(context.Background(), panelapi.SnapshotInput{Panel: p.tr.ID})
		if snap.Seq != 21 {
			t.Fatalf("after a lost event: seq %d, want 21 - the gap passed over", snap.Seq)
		}
	})

	t.Run("follow from a sequence", func(t *testing.T) {
		c := &clock{t: time.Now()}
		s := newServer(t, t.TempDir(), c, nil)
		p := &events{t: t, tr: panel.NewTrack("")}
		mustTake(t, s, openEvent(p, ""))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Current: 1}}))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Current: 2}}))

		var mu sync.Mutex
		var batches [][]int64
		stop, err := s.Follow(p.tr.ID, 1, func(es []panel.Event) {
			mu.Lock()
			defer mu.Unlock()
			var b []int64
			for _, e := range es {
				b = append(b, e.Seq)
			}
			batches = append(batches, b)
		})
		if err != nil {
			t.Fatal(err)
		}
		defer stop()
		// Five quick events: coalesced, not five deliveries.
		for i := 3; i <= 7; i++ {
			mustTake(t, s, p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Current: int64(i)}}))
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			mu.Lock()
			var all []int64
			for _, b := range batches {
				all = append(all, b...)
			}
			n := len(batches)
			mu.Unlock()
			if len(all) == 7 {
				if want := "2,3,4,5,6,7,8"; joinSeqs(all) != want {
					t.Fatalf("followed %v, want %s: after seq 1, in order, once each", all, want)
				}
				if n > 3 {
					t.Fatalf("%d deliveries for 7 events taken at once: not coalesced", n)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("followed %v, want the 7 events after seq 1", all)
			}
			time.Sleep(10 * time.Millisecond)
		}
		// History after a sequence reads the log.
		h, err := s.History(ctx, panelapi.HistoryInput{Panel: p.tr.ID, From: 5})
		if err != nil || len(h.Records) != 3 || h.Records[0].Event.Seq != 6 {
			t.Fatalf("history from 5 = %+v (%v), want events 6 to 8", h.Records, err)
		}
	})

	t.Run("a panel closed and opened again after a restart", func(t *testing.T) {
		dir := t.TempDir()
		c := &clock{t: time.Now()}
		s := newServer(t, dir, c, nil)
		p := &events{t: t, tr: panel.NewTrack("")}
		mustTake(t, s, openEvent(p, ""))
		// ops are written to the snapshot later: the restart reads the log's tail.
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindOps, Ops: []render.Op{{Op: render.OpReplace, Target: "status", Node: text("half-way")}}}))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Current: 5, Total: 10}}))
		if err := s.Input(panel.Input{Panel: p.tr.ID, Kind: panel.InputCancel}); err != nil {
			t.Fatal(err)
		}

		again := newServer(t, dir, c, nil)
		snap, err := again.Snapshot(ctx, panelapi.SnapshotInput{Panel: p.tr.ID})
		if err != nil {
			t.Fatal(err)
		}
		if snap.Seq != 3 || snap.Inputs != 1 || len(snap.Events) != 3 || snap.Events[2].Progress.Current != 5 {
			t.Fatalf("after a restart: seq %d inputs %d events %d, want seq 3, the input and open+ops+progress", snap.Seq, snap.Inputs, len(snap.Events))
		}
		// The numbering goes on from where the log is.
		mustTake(t, again, p.next(panel.Event{Kind: panel.KindComplete, Complete: &panel.Complete{Status: "ok"}}))
		snap, _ = again.Snapshot(ctx, panelapi.SnapshotInput{Panel: p.tr.ID})
		if snap.Seq != 4 || snap.State != panel.StateComplete || snap.Ended.IsZero() {
			t.Fatalf("after its end: seq %d state %s, want 4 complete with its end time", snap.Seq, snap.State)
		}
		if _, err := again.Snapshot(ctx, panelapi.SnapshotInput{Panel: panel.NewID()}); err == nil {
			t.Fatal("a panel never seen answers a snapshot")
		}
	})

	t.Run("prune after the type's retention, the waiting panel kept", func(t *testing.T) {
		dir := t.TempDir()
		c := &clock{t: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
		retention := func(_ context.Context, typ string) time.Duration {
			if typ == "ktw-test:/panel-type/short" {
				return 24 * time.Hour
			}
			return 0 // as ended runs
		}
		s := newServer(t, dir, c, retention)
		ended := func(typ string) string {
			p := &events{t: t, tr: panel.NewTrack("")}
			mustTake(t, s, openEvent(p, typ))
			mustTake(t, s, p.next(panel.Event{Kind: panel.KindComplete, Complete: &panel.Complete{Status: "ok"}}))
			return p.tr.ID
		}
		short := ended("ktw-test:/panel-type/short")
		usual := ended("")
		w := &events{t: t, tr: panel.NewTrack("")}
		mustTake(t, s, openEvent(w, ""))
		mustTake(t, s, w.next(panel.Event{Kind: panel.KindAsk, Ask: &panel.Ask{Question: "Go?"}}))
		waiting := w.tr.ID
		answered := &events{t: t, tr: panel.NewTrack("")}
		mustTake(t, s, openEvent(answered, ""))
		mustTake(t, s, answered.next(panel.Event{Kind: panel.KindAsk, Ask: &panel.Ask{Question: "Go?"}}))
		if err := s.Input(panel.Input{Panel: answered.tr.ID, Kind: panel.InputAction, Name: "go"}); err != nil {
			t.Fatal(err)
		}

		c.add(2 * 24 * time.Hour)
		pruned, err := s.Prune(ctx, c.now())
		if err != nil || strings.Join(pruned, ",") != short {
			t.Fatalf("after 2 days pruned %v (%v), want only the short-lived type's %s", pruned, err, short)
		}
		if _, err := os.Stat(filepath.Join(dir, short+".jsonl")); !os.IsNotExist(err) {
			t.Fatalf("a pruned panel's log is still there: %v", err)
		}
		c.add(30 * 24 * time.Hour)
		pruned, _ = s.Prune(ctx, c.now())
		got := map[string]bool{}
		for _, id := range pruned {
			got[id] = true
		}
		if !got[usual] || !got[answered.tr.ID] || got[waiting] {
			t.Fatalf("after a month pruned %v, want the ended panel and the answered one, never the waiting %s", pruned, waiting)
		}
		list, _ := s.List(ctx, panelapi.ListInput{})
		if len(list.Panels) != 1 || list.Panels[0].Panel != waiting || !list.Panels[0].Waiting {
			t.Fatalf("kept %+v, want the waiting panel alone", list.Panels)
		}
	})

	t.Run("the bus's events and inputs", func(t *testing.T) {
		bus := teststub.New()
		c := &clock{t: time.Now()}
		s := newServer(t, t.TempDir(), c, nil)
		stop, err := s.Start(ctx, bus, "local", "u1")
		if err != nil {
			t.Fatal(err)
		}
		defer stop()
		h, err := panel.OpenPanel(ctx, panel.Scope{Tenant: "local", Universe: "u1"}, func(ctx context.Context, subj string, b []byte) error { return bus.Publish(ctx, subj, b) }, panel.Open{Title: "Bus", Answer: render.Answer{Tree: tree()}})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.Ask(ctx, panel.Ask{Question: "Yes?"}); err != nil {
			t.Fatal(err)
		}
		in, _ := json.Marshal(panel.Input{Panel: h.ID(), Kind: panel.InputSubmit, Value: map[string]any{"ok": true}})
		waitFor(t, func() bool {
			sn, err := s.Snapshot(ctx, panelapi.SnapshotInput{Panel: h.ID()})
			return err == nil && sn.Seq == 2
		})
		if err := bus.Publish(ctx, h.InputSubject(), in); err != nil {
			t.Fatal(err)
		}
		waitFor(t, func() bool {
			sn, _ := s.Snapshot(ctx, panelapi.SnapshotInput{Panel: h.ID()})
			return sn.Inputs == 1 && !sn.Waiting
		})
		hist, _ := s.History(ctx, panelapi.HistoryInput{Panel: h.ID()})
		if len(hist.Records) != 3 || hist.Records[2].Input == nil || hist.Records[2].After != 2 {
			t.Fatalf("history = %+v, want open, ask, then the input after seq 2", hist.Records)
		}
	})

	t.Run("snapshot and history over HTTP", func(t *testing.T) {
		c := &clock{t: time.Now()}
		s := newServer(t, t.TempDir(), c, nil)
		p := &events{t: t, tr: panel.NewTrack("")}
		mustTake(t, s, openEvent(p, ""))
		mustTake(t, s, p.next(panel.Event{Kind: panel.KindProgress, Progress: &panel.Progress{Current: 1}}))
		srv := httptest.NewServer(s.Handler())
		defer srv.Close()
		var snap panelapi.Snapshot
		getJSON(t, srv.URL+"/panels/"+p.tr.ID, http.StatusOK, &snap)
		if snap.Seq != 2 || len(snap.Events) != 2 {
			t.Fatalf("HTTP snapshot = %+v, want seq 2", snap)
		}
		var hist panelapi.HistoryOutput
		getJSON(t, srv.URL+"/panels/"+p.tr.ID+"/history?from=1", http.StatusOK, &hist)
		if len(hist.Records) != 1 || hist.Records[0].Event.Seq != 2 {
			t.Fatalf("HTTP history from 1 = %+v, want event 2", hist.Records)
		}
		var list panelapi.ListOutput
		getJSON(t, srv.URL+"/panels", http.StatusOK, &list)
		if len(list.Panels) != 1 {
			t.Fatalf("HTTP list = %+v", list.Panels)
		}
		getJSON(t, srv.URL+"/panels/"+panel.NewID(), http.StatusNotFound, nil)
	})
}

func getJSON(t *testing.T, url string, status int, v any) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != status {
		t.Fatalf("GET %s = %d, want %d", url, res.StatusCode, status)
	}
	if v != nil {
		if err := json.NewDecoder(res.Body).Decode(v); err != nil {
			t.Fatal(err)
		}
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func joinSeqs(s []int64) string {
	var b []string
	for _, n := range s {
		b = append(b, itoa(n))
	}
	return strings.Join(b, ",")
}
