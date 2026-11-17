// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// gateEnv is the vocabulary a guard reads: the object's, plus who moves it
// and from where to where, the capabilities' functions bound — a costly one
// not run when noCostly (a preview).
func (s *Service) gateEnv(ctx stdctx.Context, obj api.Object, by api.Actor, from, to string, noCostly bool) map[string]any {
	env := s.objectEnv(ctx, obj, noCostly)
	env["by"] = map[string]any{"id": by.ID, "kind": by.Kind}
	env["from"], env["to"] = from, to
	return env
}

// gateSubject is the object a guard is evaluated on, with its body and fields.
func (s *Service) gateSubject(id string) (api.Object, error) {
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.Object{}, err
	}
	obj, ok := ix.get(id)
	if !ok {
		return api.Object{}, api.ObjectNotFound(id)
	}
	return s.hydrated(obj), nil
}
