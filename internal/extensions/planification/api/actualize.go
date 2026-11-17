// SPDX-License-Identifier: Apache-2.0

package api

import cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

// ACTUALIZING AN OPEN REQUEST — what was asked changes: it is rescoped (its
// wording, the old one kept), superseded by another (it ends, what depended on
// it depends on its successor) or merged into another (the same, its content
// carried over). Each says why; a running step is never rewritten, only marked
// stale.

// EdgeSupersededBy points a superseded or merged request at its successor.
const EdgeSupersededBy = "superseded-by"

// RescopeInput rewords an open request.
type RescopeInput struct {
	ID      string       `json:"id"                jsonschema:"description=The request,required"`
	Title   string       `json:"title,omitempty"   jsonschema:"description=Its new title (a request filed on a goal or a feature keeps its object's name)"`
	Details string       `json:"details,omitempty" jsonschema:"description=Its new details"`
	Note    string       `json:"note"              jsonschema:"description=Why it changes,required"`
	By      cwmapi.Actor `json:"by"                jsonschema:"description=Who rescopes it,required"`
}

// AmendDeltaInput amends one operation of a demand's delta: each field given
// replaces the operation's own; why is required.
type AmendDeltaInput struct {
	ID        string       `json:"id"                  jsonschema:"description=The demand,required"`
	Op        int          `json:"op"                  jsonschema:"description=The operation's index in its delta (from 0),required"`
	Title     string       `json:"title,omitempty"     jsonschema:"description=Its new wording"`
	Details   string       `json:"details,omitempty"   jsonschema:"description=Its new details"`
	Under     string       `json:"under,omitempty"     jsonschema:"description=The goal or feature it now goes under"`
	Module    string       `json:"module,omitempty"    jsonschema:"description=The module it now goes in"`
	Object    string       `json:"object,omitempty"    jsonschema:"description=The object it now acts on"`
	Increment string       `json:"increment,omitempty" jsonschema:"description=The increment it now aims at"`
	For       string       `json:"for,omitempty"       jsonschema:"description=The delivery goal it now answers"`
	Why       string       `json:"why"                 jsonschema:"description=Why it changes,required"`
	By        cwmapi.Actor `json:"by"                  jsonschema:"description=The person amending it,required"`
}

// SupersedeInput ends an open request for another that carries its work.
type SupersedeInput struct {
	ID   string       `json:"id"   jsonschema:"description=The request superseded,required"`
	With string       `json:"with" jsonschema:"description=The open request that carries its work from now on,required"`
	Note string       `json:"note" jsonschema:"description=Why,required"`
	By   cwmapi.Actor `json:"by"   jsonschema:"description=Who supersedes it,required"`
}

// MergeInput merges an open request into another: its details, deliverables
// and links carried over, then it ends.
type MergeInput struct {
	ID   string       `json:"id"   jsonschema:"description=The request merged,required"`
	Into string       `json:"into" jsonschema:"description=The open request it is merged into (none of its work running),required"`
	Note string       `json:"note" jsonschema:"description=Why,required"`
	By   cwmapi.Actor `json:"by"   jsonschema:"description=Who merges it,required"`
}
