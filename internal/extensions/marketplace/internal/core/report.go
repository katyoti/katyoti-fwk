// SPDX-License-Identifier: Apache-2.0

package core

// THE LONG METHODS REPORT THEIR STEPS. Install, Update
// SetVersion, InstallSolution, InstallPrereqs and Doctor are declared long
// (SetupServiceDef): run from the action surface each is a method run that
// owns a panel, lent to this process over the bus (ktw.meta.panel), and each
// reports its steps into it through the neutral progress contract -
// progress.Emit of a panel.Progress: what starts or ends, how many of its
// steps are done. Only the outermost of them reports: an update installs
// each item it moves, a solution installs each member, and those installs
// add no bar of their own to the one already running. With no reporter on
// the context (the command line, a test) nothing is sent.

import (
	"context"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
)

type stepsKey struct{}

// steps is one long method's report: done of total steps.
type steps struct {
	ctx   context.Context
	on    bool
	mu    sync.Mutex
	done  int64
	total int64
}

// beginSteps starts the report of a long method of total steps (0: not
// known yet) and answers the context its inner calls run with: a method
// called inside another one's report reports nothing.
func beginSteps(ctx context.Context, total int) (context.Context, *steps) {
	st := &steps{ctx: ctx, total: int64(total)}
	if _, nested := ctx.Value(stepsKey{}).(*steps); nested {
		return ctx, st
	}
	_, st.on = progress.EmitterFrom(ctx)
	return context.WithValue(ctx, stepsKey{}, st), st
}

// setTotal is the number of steps, once known.
func (s *steps) setTotal(total int) {
	s.mu.Lock()
	s.total = int64(total)
	s.mu.Unlock()
}

// begin says a step starts.
func (s *steps) begin(label string) {
	s.mu.Lock()
	done, total := s.done, s.total
	s.mu.Unlock()
	s.emit(label, done, total)
}

// end says a step ended.
func (s *steps) end(label string) {
	s.mu.Lock()
	s.done++
	done, total := s.done, s.total
	s.mu.Unlock()
	s.emit(label, done, total)
}

func (s *steps) emit(label string, done, total int64) {
	if !s.on {
		return
	}
	if total < done {
		total = 0 // more steps than announced: an open bar rather than a wrong one
	}
	progress.Emit(s.ctx, panel.Progress{Label: label, Current: done, Total: total})
}

// stepWriter is an io.Writer whose every line is a step that ends: the
// prerequisites' engine says each program as it starts.
type stepWriter struct{ st *steps }

func (w stepWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(string(p), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			w.st.end(line)
		}
	}
	return len(p), nil
}
