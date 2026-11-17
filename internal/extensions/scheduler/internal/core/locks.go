// SPDX-License-Identifier: Apache-2.0

package core

import (
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// MODULE LOCKS — what running work holds on the modules: a writer runs alone on
// its module, readers share; a module's lock is that module's only, never its
// sub-modules': a step that changes its sub-modules too lists them in its
// writes, one that rolls them up lists them in its reads (
// which superseded OD-004's subtree lock for writers). The queue takes them.
// A lock is keyed (repository, module): one store serves several projects,
// and two projects' modules of one name never hold each other.

// lock is a module of a repository held by a running to-do, or reserved by a
// ready one ahead of it in the queue: read, or written. by is that to-do,
// claimant the worker holding it and request its request (a pull's
// structured wait names them).
type lock struct {
	repository string
	module     string
	write      bool
	by         string
	reserved   bool
	claimant   *cwmapi.Actor
	request    string
}

// locks is what running to-dos hold.
type locks struct {
	held []lock
}

// clash: a and b cannot be held at once — one module of one repository,
// unless both read.
func (l *locks) clash(a, b lock) bool {
	return a.repository == b.repository && a.module == b.module && (a.write || b.write)
}

// conflict is why a step reading reads and writing writes cannot take its
// locks; "" when it can.
func (l *locks) conflict(reads, writes []string) string { return l.conflictIn("", reads, writes) }

// conflictIn is conflict for a step of repository.
func (l *locks) conflictIn(repository string, reads, writes []string) string {
	why, _ := l.waitIn(repository, reads, writes)
	return why
}

// waitIn is conflictIn with its structured wait (cause lock or reserved, the
// module, the to-do behind it with its worker and request); nil when it can
// take its locks.
func (l *locks) waitIn(repository string, reads, writes []string) (string, *api.QueueWait) {
	asked := make([]lock, 0, len(writes)+len(reads))
	for _, m := range writes {
		asked = append(asked, lock{repository: repository, module: m, write: true})
	}
	for _, m := range reads {
		asked = append(asked, lock{repository: repository, module: m})
	}
	for _, a := range asked {
		for _, h := range l.held {
			if !l.clash(a, h) {
				continue
			}
			why := "module " + a.module + " is being changed"
			if a.write {
				why = "module " + a.module + " is in use"
			}
			w := &api.QueueWait{Cause: api.WaitLock, Module: a.module, HolderTodo: h.by, HolderRequest: h.request}
			if h.reserved {
				w.Cause = api.WaitReserved
			}
			if h.claimant != nil {
				c := *h.claimant
				w.Holder = &c
			}
			return why + h.owner(), w
		}
	}
	return "", nil
}

// owner names the to-do behind a lock, "" when none is known: the one that
// holds it, or the ready one ahead in the queue that reserves it
func (h lock) owner() string {
	switch {
	case h.by == "":
		return ""
	case h.reserved:
		return " (reserved by " + h.by + ", ready ahead of it in the queue)"
	}
	return " (held by " + h.by + ")"
}

func (l *locks) take(reads, writes []string) { l.takeIn("", reads, writes) }

// takeIn holds the modules of repository a step reads and writes.
func (l *locks) takeIn(repository string, reads, writes []string) {
	l.add(repository, reads, writes, lock{})
}

// holdFor holds the modules the running to-do t reads and writes; reserveFor
// reserves them for the ready to-do t, ahead in the queue.
func (l *locks) holdFor(t api.Todo) {
	h := lock{by: t.ID, request: t.Request}
	if t.Claim != nil {
		c := t.Claim.By
		h.claimant = &c
	}
	l.add(t.Repository, t.Reads, t.Writes, h)
}

func (l *locks) reserveFor(t api.Todo) {
	l.add(t.Repository, t.Reads, t.Writes, lock{by: t.ID, request: t.Request, reserved: true})
}

// add holds reads and writes of repository, each lock a copy of h.
func (l *locks) add(repository string, reads, writes []string, h lock) {
	h.repository = repository
	for _, m := range reads {
		r := h
		r.module, r.write = m, false
		l.held = append(l.held, r)
	}
	for _, m := range writes {
		w := h
		w.module, w.write = m, true
		l.held = append(l.held, w)
	}
}
