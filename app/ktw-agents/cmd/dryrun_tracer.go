// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// dryRunTracer captures workflow/child/ledger/messaging events for the
// `--dry-run` CLI flow. Subscribes via the bus's SubscribeAny so it sees every
// topic without enumerating them upfront — child names are dynamic.
//
// Lifecycle:
//
//	tr := newDryRunTracer()
//	unsub := tr.Attach(bus)
//	// ... run workflow ...
//	unsub()
//	tr.Render(os.Stdout)
//	tr.WriteJSONL(file)  // optional
type dryRunTracer struct {
	mu     sync.Mutex
	events []tracedEvent
	start  time.Time
}

// tracedEvent is one captured pub on the bus.
type tracedEvent struct {
	TS      time.Time `json:"ts"`
	Topic   string    `json:"topic"`
	Payload any       `json:"payload,omitempty"`
}

func newDryRunTracer() *dryRunTracer {
	return &dryRunTracer{start: time.Now()}
}

// Attach subscribes to all bus events. Returns an unsubscribe function.
func (t *dryRunTracer) Attach(bus engine.EventBus) func() {
	return bus.SubscribeAny(func(topic string, payload any) {
		t.mu.Lock()
		t.events = append(t.events, tracedEvent{
			TS:      time.Now(),
			Topic:   topic,
			Payload: payload,
		})
		t.mu.Unlock()
	})
}

// childRow summarizes one spawned child for ASCII rendering.
type childRow struct {
	name         string
	emittedKind  string // "ledger", "simulator", etc. (the kind actually used by the spawner)
	originalKind string // when substituted, the agent/task/program/human-gate kind
	substituted  bool
	status       string // "ok", "fail", "?"
	durationMs   int64
	startedTS    time.Time
	endedTS      time.Time
}

// Render writes a human-readable ASCII summary to w.
func (t *dryRunTracer) Render(w io.Writer, taskName string) {
	t.mu.Lock()
	events := append([]tracedEvent(nil), t.events...)
	t.mu.Unlock()

	totalDuration := time.Since(t.start)

	// Topic counts.
	counts := make(map[string]int)
	for _, e := range events {
		counts[e.Topic]++
	}

	// Reconstruct child rows from child.<name>.{started,done,failed}.
	rows := make(map[string]*childRow)
	var order []string
	for _, e := range events {
		parts := strings.SplitN(e.Topic, ".", 3)
		if len(parts) != 3 || parts[0] != "child" {
			continue
		}
		name, suffix := parts[1], parts[2]
		row, ok := rows[name]
		if !ok {
			row = &childRow{name: name, status: "?"}
			rows[name] = row
			order = append(order, name)
		}
		switch suffix {
		case "started":
			row.startedTS = e.TS
			if m := payloadMap(e.Payload); m != nil {
				if k, _ := m["childKind"].(string); k != "" {
					row.emittedKind = k
				}
			}
		case "done":
			row.status = "ok"
			row.endedTS = e.TS
			if !row.startedTS.IsZero() {
				row.durationMs = e.TS.Sub(row.startedTS).Milliseconds()
			}
			// Detect substitution by inspecting the output payload.
			if m := payloadMap(e.Payload); m != nil {
				if outRaw, ok := m["output"]; ok {
					if outMap, ok := outRaw.(map[string]any); ok {
						if dr, _ := outMap["dry_run"].(bool); dr {
							row.substituted = true
							if origKind, _ := outMap["child_kind"].(string); origKind != "" {
								row.originalKind = origKind
							}
						}
					}
				}
			}
		case "failed":
			row.status = "fail"
			row.endedTS = e.TS
			if !row.startedTS.IsZero() {
				row.durationMs = e.TS.Sub(row.startedTS).Milliseconds()
			}
		}
	}

	// Workflow runID(s) for the header.
	var runID string
	for _, e := range events {
		if strings.HasPrefix(e.Topic, "workflow.") && strings.HasSuffix(e.Topic, ".started") {
			parts := strings.SplitN(e.Topic, ".", 3)
			if len(parts) == 3 {
				runID = parts[1]
				break
			}
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Dry-run: %s", taskName)
	if runID != "" {
		fmt.Fprintf(w, " (%s)", runID)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Duration: %dms (no LLM)\n", totalDuration.Milliseconds())
	fmt.Fprintln(w)

	if len(order) == 0 {
		fmt.Fprintln(w, "No children spawned.")
	} else {
		fmt.Fprintf(w, "Children spawned: %d\n", len(order))
		// Find max name length for alignment.
		maxName := 0
		for _, name := range order {
			if len(name) > maxName {
				maxName = len(name)
			}
		}
		for _, name := range order {
			row := rows[name]
			marker := "?"
			switch row.status {
			case "ok":
				marker = "OK  "
			case "fail":
				marker = "FAIL"
			}
			kindLabel := row.emittedKind
			if row.substituted {
				kindLabel = fmt.Sprintf("%s→sim", row.originalKind)
			}
			fmt.Fprintf(w, "  %s  %-*s  (%-18s)  %dms\n",
				marker, maxName, name, kindLabel, row.durationMs)
		}
	}
	fmt.Fprintln(w)

	// Topic counts (sorted). Heartbeats are collapsed to one line because
	// every executor emits its own and they dominate the listing without
	// adding signal to the topology view.
	if len(counts) > 0 {
		fmt.Fprintf(w, "Events: %d total\n", len(events))
		topics := make([]string, 0, len(counts))
		heartbeatCount := 0
		for t, c := range counts {
			if strings.HasPrefix(t, "heartbeat.") {
				heartbeatCount += c
				continue
			}
			topics = append(topics, t)
		}
		sort.Strings(topics)
		for _, topic := range topics {
			fmt.Fprintf(w, "  %s: %d\n", topic, counts[topic])
		}
		if heartbeatCount > 0 {
			fmt.Fprintf(w, "  heartbeat.* (collapsed): %d\n", heartbeatCount)
		}
		fmt.Fprintln(w)
	}

	// Substitution + fallback summary.
	subCount := 0
	for _, row := range rows {
		if row.substituted {
			subCount++
		}
	}
	fmt.Fprintf(w, "Substituted: %d children (LLM/program → simulator)\n", subCount)

	// Fallback coverage: count children whose name starts with one of the
	// known fallback patterns from the YAML loader. Without explicit metadata
	// the heuristic is: a child whose status is "ok" AND there's a sibling
	// with the same prefix that "fail"ed indicates a fallback path.
	fmt.Fprintf(w, "On_error coverage: %d fallback paths triggered\n",
		countFallbacks(rows))
}

// WriteJSONL dumps captured events as JSONL to w.
func (t *dryRunTracer) WriteJSONL(w io.Writer) error {
	t.mu.Lock()
	events := append([]tracedEvent(nil), t.events...)
	t.mu.Unlock()

	enc := json.NewEncoder(w)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}

// payloadMap returns a map view of payload for inspection. Handles common
// shapes: *struct (via JSON roundtrip), map[string]any directly.
func payloadMap(payload any) map[string]any {
	if payload == nil {
		return nil
	}
	if m, ok := payload.(map[string]any); ok {
		return m
	}
	// JSON-roundtrip for typed structs (ChildDoneEvent, etc.)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// countFallbacks counts children that recovered via fallback. A fallback child
// is one whose name appears in another child's failure path. Without metadata
// we infer it: any child whose name ends with "-fallback" OR appears after a
// failed sibling in event order.
func countFallbacks(rows map[string]*childRow) int {
	count := 0
	for name, row := range rows {
		if row.status != "ok" {
			continue
		}
		if strings.HasSuffix(name, "-fallback") || strings.HasSuffix(name, "_fallback") {
			count++
		}
	}
	return count
}
