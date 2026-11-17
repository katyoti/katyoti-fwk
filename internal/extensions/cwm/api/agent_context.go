// SPDX-License-Identifier: Apache-2.0

package api

// THE AGENT CONTEXT OF AN OBJECT — what a chat started on it receives: ordered
// system sections, tools, skills to advertise, and named files read as context.
//
// Every contributor declares the same `agent:` block. They are layered in one
// order, farthest first: the project (workspace.yaml), the rules in force, the
// extensions whose objects these are, every capability in force, the object's
// type, the object itself. A closer entry with the same key REPLACES a farther
// one (keeping the farther one's place); a closer entry whose `when` is the
// literal "false" REMOVES it. What survives is filtered by its `when` and its
// text interpolated (`${...}`, el over the object vocabulary) — an expression
// that cannot be read is REPORTED on the entry, never injected as a template.
//
// A size budget bounds the system sections; over it, sections are trimmed
// farthest first and every trim is reported, so the chat still starts and the
// person can see what it did not receive.

// AgentDecl is one contributor's declaration.
type AgentDecl struct {
	System []AgentSection `json:"system,omitempty" yaml:"system,omitempty" jsonschema:"System-prompt sections; keyed by id"`
	Tools  []AgentTool    `json:"tools,omitempty"  yaml:"tools,omitempty"  jsonschema:"Tools the agent receives; keyed by uri (or plugin)"`
	Skills []AgentSkill   `json:"skills,omitempty" yaml:"skills,omitempty" jsonschema:"Skill selectors to advertise; keyed by selector"`
	Files  []AgentFile    `json:"files,omitempty"  yaml:"files,omitempty"  jsonschema:"Named files of the object read as context sections; keyed by capability and name"`
}

// IsZero reports a declaration that contributes nothing.
func (d *AgentDecl) IsZero() bool {
	return d == nil || len(d.System)+len(d.Tools)+len(d.Skills)+len(d.Files) == 0
}

// AgentSection is one system-prompt section.
type AgentSection struct {
	ID    string `json:"id"              yaml:"id"              jsonschema:"Stable id; a closer section with the same id replaces this one"`
	Title string `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"Heading shown above the text"`
	Text  string `json:"text,omitempty"  yaml:"text,omitempty"  jsonschema:"The section; ${...} is el over the object vocabulary"`
	Order int    `json:"order,omitempty" yaml:"order,omitempty" jsonschema:"Sort key; equal orders keep the layer order"`
	When  string `json:"when,omitempty"  yaml:"when,omitempty"  jsonschema:"el predicate; the literal false removes a farther section of the same id"`
}

// AgentTool is one tool: an action URI, or a plugin ref that brings its tools.
type AgentTool struct {
	URI    string `json:"uri,omitempty"    yaml:"uri,omitempty"    jsonschema:"Action URI (<provider>:<slot>/<method> or a task)"`
	Plugin string `json:"plugin,omitempty" yaml:"plugin,omitempty" jsonschema:"Plugin ref (workspace / bash / group:standard); exactly one of uri and plugin"`
	When   string `json:"when,omitempty"   yaml:"when,omitempty"   jsonschema:"el predicate; the literal false removes a farther tool of the same key"`
}

// Key is what a closer declaration overrides by.
func (t AgentTool) Key() string {
	if t.URI != "" {
		return t.URI
	}
	return "plugin:" + t.Plugin
}

// AgentSkill is one skill selector (exact name, prefix glob, <provider>:/skill/*).
type AgentSkill struct {
	Selector string `json:"selector"       yaml:"selector"       jsonschema:"Skill selector"`
	When     string `json:"when,omitempty" yaml:"when,omitempty" jsonschema:"el predicate; the literal false removes a farther selector"`
}

// AgentFile names one named file of the object to read into the context.
type AgentFile struct {
	Capability string `json:"capability,omitempty" yaml:"capability,omitempty" jsonschema:"Owning capability; empty = the object's own file"`
	Name       string `json:"name"                 yaml:"name"                 jsonschema:"The file's name"`
	Title      string `json:"title,omitempty"      yaml:"title,omitempty"      jsonschema:"Heading; default capability/name"`
	Order      int    `json:"order,omitempty"      yaml:"order,omitempty"      jsonschema:"Sort key among the sections"`
	When       string `json:"when,omitempty"       yaml:"when,omitempty"       jsonschema:"el predicate; the literal false removes a farther entry"`
}

// Key is what a closer declaration overrides by.
func (f AgentFile) Key() string { return f.Capability + "/" + f.Name }
