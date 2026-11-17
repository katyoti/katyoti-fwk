// SPDX-License-Identifier: Apache-2.0

package core

// Helpers the object verbs share: the sidecar metadata beside a module file,
// the capability-check probe cache, the action and type-choice helpers.

import (
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

const metaSuffix = ".meta.yaml"

// readSidecar loads `<full>.meta.yaml` if present; returns nil otherwise.
func readSidecar(full string) *api.FileMeta {
	mb, err := atomicio.ReadAtomic(full + metaSuffix)
	if err != nil {
		return nil
	}
	var m api.FileMeta
	if marshal.Unmarshal(mb, marshal.YAML, &m) != nil {
		return nil
	}
	return &m
}

// sliceContains reports whether s holds target.
func sliceContains(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

// matchTypes reports whether name is targeted by types — `*` is a wildcard.
func matchTypes(types []string, name string) bool {
	for _, t := range types {
		if t == "*" || t == name {
			return true
		}
	}
	return false
}

// sortChoices sorts a TypeChoice slice kebab-stably by Name.
func sortChoices(c []api.TypeChoice) {
	sort.Slice(c, func(i, j int) bool { return c[i].Name < c[j].Name })
}

// ancestorsOf returns the parent chain, closest first, excluding the object.
// A cycle stops the walk: containment is authored, so a file can name its own
// descendant as its parent, and the containment lint is what reports it.
func (s *Service) ancestorsOf(id string) []string {
	ix := s.index()
	var chain []string
	seen := map[string]bool{id: true}
	cur := id
	for {
		obj, ok := ix.get(cur)
		if !ok {
			return chain
		}
		parent := obj.ParentID()
		if parent == "" || seen[parent] {
			return chain
		}
		seen[parent] = true
		chain = append(chain, parent)
		cur = parent
	}
}
