// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tui"
	termadapter "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/adapter"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
	termserver "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/server"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/translator"
)

// runConsole executes the task while rendering a READ-ONLY multi-pane console:
// one pane per task run — the parent AND every delegated child — each pane the
// existing terminal-protocol transcoding fed by a per-run STRICT translator.
//
// All runs publish on one shared EventBus, so a single EventTaskStarted
// subscription discovers the whole delegation tree as it unfolds; each new run
// gets its own session + strict translator + transport + pane. The console is
// read-only (it never answers prompts) — it suits headless / delegated runs
// (the agent-as-human test harness, background work). A run that needs
// interactive user input should use plain `run` (the single TUI) instead.
func runConsole(ctx context.Context, rc *runConfig) error {
	bus, _ := application.UseService[engine.EventBus](ctx)
	if bus == nil {
		return fmt.Errorf("--console: no EventBus service available")
	}

	// ONE Room PER RUN. A Room's state-sync replays the WHOLE room's history —
	// sendSyncFrom is not session-filtered — so sharing a room would replay an
	// earlier run's transcript into a later-connecting pane (the cross-pane
	// leak). A fresh room per run keeps each pane's history strictly its own.
	// Rooms persist to a temp dir (read-only console → no need to litter the cwd
	// with ./terminal/) and are closed on exit.
	consoleTmp := filepath.Join(os.TempDir(), "ktw-console")
	var rooms []*room.Room
	var roomsMu sync.Mutex
	defer func() {
		roomsMu.Lock()
		for _, rm := range rooms {
			rm.Close()
		}
		roomsMu.Unlock()
	}()

	panes := make(chan tui.PaneSpec, 32)
	done := make(chan struct{})

	// Discover each run as it starts and stand up a pane for it.
	var mu sync.Mutex
	seen := map[string]bool{}
	unsub := bus.SubscribeAny(func(topic string, payload any) {
		if topic != engine.EventTaskStarted {
			return
		}
		e, ok := asTaskStarted(payload)
		if !ok {
			return
		}
		mu.Lock()
		if seen[e.RunID] {
			mu.Unlock()
			return
		}
		seen[e.RunID] = true
		mu.Unlock()

		// Key the console session by RUN id, NOT the run's real session: an
		// agent_as_human CHILD shares its PARENT's session, so a per-session pane
		// would merge a parent and its children into one stream. The run id is
		// unique per run → one isolated room + session + pane each. The translator
		// stamps its output with this id and scopes intake to this run's events
		// via Own (the run's events carry emitter == RunID).
		consoleSess := e.RunID
		runRoom := room.NewRoom(consoleTmp, nil)
		runRoom.SetBus(bus)
		roomsMu.Lock()
		rooms = append(rooms, runRoom)
		roomsMu.Unlock()
		_, _ = runRoom.CreateSession(consoleSess, e.TaskName, "")
		serverT, clientT := termadapter.NewChannelPair(256)
		if err := runRoom.Connect(ctx, serverT, consoleSess); err != nil {
			return
		}
		trans := translator.NewSessionScoped(bus, runRoom, consoleSess)
		trans.Own(e.RunID)
		trans.Start()

		label := e.TaskName
		if label == "" {
			label = consoleShortID(e.RunID)
		}
		select {
		case panes <- tui.PaneSpec{RunID: e.RunID, Label: label, Transport: clientT}:
		case <-ctx.Done():
		}
	})
	defer unsub()

	// Run the task headless. ServeTask owns session/gate setup; its own
	// transport output is irrelevant here (the panes render from the bus), so we
	// drain it. When the task tree finishes, ServeTask returns → close(done).
	go func() {
		serverT, clientT := termadapter.NewChannelPair(256)
		go consoleDrain(ctx, clientT)
		opts := buildTaskOpts(rc)
		opts.SkipHumanGate = true
		_ = termserver.ServeTask(ctx, serverT, opts)
		serverT.Close()
		close(done)
	}()

	return tui.RunMultiPaneConsole(ctx, panes, done)
}

// asTaskStarted normalizes the EventTaskStarted payload (value or pointer).
func asTaskStarted(payload any) (*engine.TaskStartedEvent, bool) {
	switch e := payload.(type) {
	case *engine.TaskStartedEvent:
		return e, true
	case engine.TaskStartedEvent:
		return &e, true
	}
	return nil, false
}

func consoleShortID(id string) string {
	if len(id) > 6 {
		return id[len(id)-6:]
	}
	return id
}

// consoleDrain empties a transport's receive channel so the producing ServeTask
// never blocks on a full buffer (the console renders from the bus, not here).
func consoleDrain(ctx context.Context, t *termadapter.Channel) {
	ch, err := t.Receive(ctx)
	if err != nil {
		return
	}
	for range ch {
	}
}
