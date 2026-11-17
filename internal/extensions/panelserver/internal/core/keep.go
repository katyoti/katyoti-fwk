// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
)

// Keeping the outcome: a panel whose type keeps it
// (panel.TypeDecl.Keep: outcome or full), or one the person asked to keep,
// leaves its final state and the person's inputs as one document on the
// object its open named, once it ends. The log itself stays in the server's
// folder, never in a module.

// keepMode is what the panel keeps now: "" while there is nothing to write -
// it has not ended, it names no object, its outcome is written already, or
// neither its type nor the person keeps it.
func (s *Server) keepMode(ctx context.Context, sn panelapi.Snapshot) string {
	if s.opts.Keep == nil || sn.Object == "" || sn.Kept != "" || sn.Ended.IsZero() {
		return ""
	}
	mode := panel.KeepNone
	if s.opts.KeepOf != nil && sn.Type != "" {
		if k := s.opts.KeepOf(ctx, sn.Type); k != "" {
			mode = k
		}
	}
	if mode == panel.KeepNone && sn.KeepAsked {
		mode = panel.KeepOutcome
	}
	if mode != panel.KeepOutcome && mode != panel.KeepFull {
		return ""
	}
	return mode
}

// keepLocked starts writing the entry's outcome when it is due and no write
// of it runs: off the panel's lock, so the panel takes its events meanwhile.
// It reports whether a write started. The caller holds en.mu.
func (s *Server) keepLocked(en *entry) bool {
	if en.keeping {
		return false
	}
	ctx := s.baseCtx()
	mode := s.keepMode(ctx, en.snap)
	if mode == "" {
		return false
	}
	records, err := s.st.readLog(en.snap.Panel)
	if err != nil {
		slog.Warn("panel server: outcome not kept - its log does not read", "panel", en.snap.Panel, "err", err)
		return false
	}
	o := panelapi.Outcome{Snapshot: copySnapshot(en.snap), Keep: mode}
	for _, r := range records {
		if r.Input != nil {
			o.Inputs = append(o.Inputs, r)
		}
	}
	if mode == panel.KeepFull {
		o.Log = records
	}
	en.keeping = true
	s.keeps.Add(1)
	go s.writeOutcome(ctx, en, o)
	return true
}

// writeOutcome hands the outcome to the keeper and records where it went; a
// keeper that fails leaves it to be tried again at the next prune pass.
func (s *Server) writeOutcome(ctx context.Context, en *entry, o panelapi.Outcome) {
	defer s.keeps.Done()
	path, err := s.opts.Keep(ctx, o.Snapshot.Object, o)
	en.mu.Lock()
	defer en.mu.Unlock()
	en.keeping = false
	if err != nil {
		slog.Warn("panel server: outcome not kept - tried again at the next prune", "panel", o.Snapshot.Panel, "object", o.Snapshot.Object, "err", err)
		return
	}
	en.snap.Kept = path
	if err := s.saveLocked(en, true); err != nil {
		slog.Warn("panel server: snapshot not written", "panel", o.Snapshot.Panel, "err", err)
	}
}

// WaitKeeps waits for the outcomes being written.
func (s *Server) WaitKeeps() { s.keeps.Wait() }

// Keep takes the person's ask to keep a panel's outcome, whatever its type
// keeps: written now when the panel has ended, else when it ends.
func (s *Server) Keep(_ context.Context, in panelapi.KeepInput) (panelapi.KeepOutput, error) {
	if !panel.ValidID(in.Panel) {
		return panelapi.KeepOutput{}, fmt.Errorf("panel server: %q is no panel id", in.Panel)
	}
	en, err := s.panel(in.Panel)
	if err != nil {
		return panelapi.KeepOutput{}, err
	}
	en.mu.Lock()
	defer en.mu.Unlock()
	if !en.known {
		return panelapi.KeepOutput{}, unknownPanel(in.Panel)
	}
	if en.snap.Kept != "" {
		return panelapi.KeepOutput{Kept: en.snap.Kept}, nil
	}
	if en.snap.Object == "" {
		return panelapi.KeepOutput{}, fmt.Errorf("panel server: panel %s names no object: its outcome has nowhere to be kept", in.Panel)
	}
	if !en.snap.KeepAsked {
		en.snap.KeepAsked = true
		if err := s.saveLocked(en, true); err != nil {
			return panelapi.KeepOutput{}, err
		}
	}
	s.keepLocked(en)
	return panelapi.KeepOutput{Pending: true}, nil
}

// DocumentPath is where a panel's outcome is written in its object's module.
func DocumentPath(panelID string) string {
	return panelapi.DocumentFolder + "/" + panelID + ".md"
}

// OutcomeDocument is the document a kept panel leaves on its object: its
// identity as front matter, then its result, how far it went, the person's
// inputs and the events a screen folds to draw it as it ended - and, for a
// type that keeps it all, its whole log.
func OutcomeDocument(o panelapi.Outcome) ([]byte, error) {
	sn := o.Snapshot
	status := ""
	var result any
	var progress *panel.Progress
	for _, e := range sn.Events {
		switch {
		case e.Kind == panel.KindComplete && e.Complete != nil:
			status, result = e.Complete.Status, e.Complete.Result
		case e.Kind == panel.KindProgress && e.Progress != nil:
			progress = e.Progress
		}
	}
	if status == "" {
		status = string(sn.State)
	}
	meta := map[string]any{
		"panel":  sn.Panel,
		"object": sn.Object,
		"status": status,
		"keep":   o.Keep,
		"inputs": len(o.Inputs),
	}
	for k, v := range map[string]string{"type": sn.Type, "title": sn.Title, "owner": sn.Owner, "parent": sn.Parent} {
		if v != "" {
			meta[k] = v
		}
	}
	for k, t := range map[string]time.Time{"opened": sn.Opened, "ended": sn.Ended} {
		if !t.IsZero() {
			meta[k] = t.UTC().Format(time.RFC3339)
		}
	}
	front, err := marshal.Marshal(meta, marshal.YAML)
	if err != nil {
		return nil, fmt.Errorf("panel server: outcome of %s: %w", sn.Panel, err)
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(front)
	if !bytes.HasSuffix(front, []byte("\n")) {
		b.WriteString("\n")
	}
	b.WriteString("---\n\n")
	title := sn.Title
	if title == "" {
		title = "Panel " + sn.Panel
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	if !sn.Ended.IsZero() {
		fmt.Fprintf(&b, "Ended %s: %s.\n\n", sn.Ended.UTC().Format(time.RFC3339), status)
	}
	if result != nil {
		b.WriteString("## Result\n\n")
		if err := jsonBlock(&b, result); err != nil {
			return nil, fmt.Errorf("panel server: outcome of %s: %w", sn.Panel, err)
		}
	}
	if progress != nil {
		b.WriteString("## Progress\n\n")
		line := progress.Label
		if progress.Total > 0 {
			line = strings.TrimSpace(fmt.Sprintf("%s %d of %d", line, progress.Current, progress.Total))
		}
		fmt.Fprintf(&b, "%s\n\n", line)
	}
	b.WriteString("## The person's inputs\n\n")
	if len(o.Inputs) == 0 {
		b.WriteString("None.\n\n")
	}
	for _, r := range o.Inputs {
		in := r.Input
		parts := []string{r.At.UTC().Format(time.RFC3339), string(in.Kind)}
		if in.Name != "" {
			parts = append(parts, in.Name)
		}
		if in.Text != "" {
			parts = append(parts, fmt.Sprintf("%q", in.Text))
		}
		if in.Value != nil {
			v, err := json.Marshal(in.Value)
			if err != nil {
				return nil, fmt.Errorf("panel server: outcome of %s: %w", sn.Panel, err)
			}
			parts = append(parts, "`"+string(v)+"`")
		}
		if in.By != "" {
			parts = append(parts, "by "+in.By)
		}
		fmt.Fprintf(&b, "- %s\n", strings.Join(parts, " - "))
	}
	if len(o.Inputs) > 0 {
		b.WriteString("\n")
	}
	b.WriteString("## Final view\n\nThe events a screen folds to draw the panel as it ended.\n\n")
	if err := jsonBlock(&b, sn.Events); err != nil {
		return nil, fmt.Errorf("panel server: outcome of %s: %w", sn.Panel, err)
	}
	if o.Keep == panel.KeepFull {
		b.WriteString("## Log\n\nEach event out and each input in, in the order taken.\n\n```jsonl\n")
		for _, r := range o.Log {
			line, err := json.Marshal(r)
			if err != nil {
				return nil, fmt.Errorf("panel server: outcome of %s: %w", sn.Panel, err)
			}
			b.Write(line)
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}
	return b.Bytes(), nil
}

func jsonBlock(b *bytes.Buffer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b.WriteString("```json\n")
	b.Write(data)
	b.WriteString("\n```\n\n")
	return nil
}
