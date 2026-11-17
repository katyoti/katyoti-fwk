// SPDX-License-Identifier: Apache-2.0

package api

// This file is the DESCRIPTOR vocabulary: the shapes that describe a service
// surface — its slots, methods, schemas and CLI presentation.
//
// It belongs with the consumer contract because describing what can be called is
// a consumer concern: a client, a catalogue, a documentation generator and a GUI
// all need these shapes without needing the machinery that serves them. The
// types are pure data with no dependencies, so placing them here costs nothing
// and keeps the protocol implementation out of every consumer's closure.
//
// The protocol package re-exports every name, so existing callers are unaffected.

// SlotType identifies the capability contract a slot implements.
// Standard types are defined as constants; custom sidecar types use prefixed names.
type SlotType string

const (
	SlotTypeAgent        SlotType = "agent"
	SlotTypeSkill        SlotType = "skill"
	SlotTypeUserPrompter SlotType = "user_prompter"
	SlotTypeKTSP         SlotType = "ktsp"
)

// SlotTags and MethodTags are open-ended key-value metadata.
// Generators read the tags they care about, ignore the rest.
type SlotTags map[string]string
type MethodTags map[string]string

// Standard tag keys for SlotTags.
const (
	TagSlotInternal  = "internal"   // "true" — slot is not discoverable
	TagSlotCLI       = "cli"        // "false" — skip CLI generation for this slot
	TagSlotCLIFlat   = "cli.flat"   // "true" — methods appear as top-level commands
	TagSlotCLIName   = "cli.name"   // override slot name in CLI
	TagSlotCLIAlias  = "cli.alias"  // alias for slot command
	TagSlotCLIGroups = "cli.groups" // JSON-encoded []meta.CLIGroup for noun-verb command grouping
	TagSlotMCP       = "mcp"        // "false" — skip MCP generation for this slot
	TagSlotMCPPrefix = "mcp.prefix" // override slot prefix in MCP tool names
	// TagSlotOrdered "true" — the bus serves the slot's requests one at a
	// time, in arrival order (by default it serves them concurrently).
	TagSlotOrdered = "ordered"
	// TagSlotMaxConcurrent "<n>" — how many of the slot's requests the bus
	// serves at once (messagebus.DefaultMaxConcurrent when unset).
	TagSlotMaxConcurrent = "max-concurrent"
)

// Standard tag keys for MethodTags.
const (
	TagMethodCLI       = "cli"        // "false" — skip CLI generation for this method
	TagMethodCLIName   = "cli.name"   // override method name in CLI
	TagMethodCLIHidden = "cli.hidden" // "true" — hide from CLI help
	TagMethodCLIAlias  = "cli.alias"  // alias for method command
	TagMethodMCP       = "mcp"        // "false" — skip MCP generation for this method
	TagMethodMCPName   = "mcp.name"   // override method name in MCP
	// TagMethodReadOnly marks a method that changes nothing: a caller may run
	// it to observe state (a guide recipe's check) and it leaves no
	// invocation record. Set from meta.MethodDef.ReadOnly.
	TagMethodReadOnly = "read-only"
	// TagMethodLong marks a method that runs longer than a person waits for:
	// the action surface runs it as a method run with its panel, answering
	// both at once. Set from meta.MethodDef.LongRunning.
	TagMethodLong = "long"
)

// CLIArg describes a positional CLI argument derived from a struct field.
type CLIArg struct {
	Field    string   `json:"field"`
	Desc     string   `json:"description"`
	Required bool     `json:"required,omitempty"`
	Enum     []string `json:"enum,omitempty"`
}

// CLIFlag describes a CLI flag derived from a struct field.
type CLIFlag struct {
	Field string `json:"field"`
	// Name is the CLI flag name (e.g. "dry-run"). It may differ from the
	// JSON key when a `cli:` tag renames the flag.
	Name string `json:"name"`
	// JSONKey is the params key the receiving struct expects (the json tag,
	// e.g. "dry_run"). Distinct from Name so a `cli:` tag can give a kebab
	// flag while the payload still lands under the snake_case json field.
	// Falls back to Name when empty.
	JSONKey  string   `json:"json_key,omitempty"`
	Short    string   `json:"short,omitempty"`
	Desc     string   `json:"description"`
	Type     string   `json:"type"`
	Default  string   `json:"default,omitempty"`
	Required bool     `json:"required,omitempty"`
	Enum     []string `json:"enum,omitempty"`
	// Path marks a folder or file the person types (a `path:"cwd"` tag): the
	// command line resolves it against its working directory — empty is that
	// directory — before sending it, where a chat's relative path is the
	// method's to anchor.
	Path bool `json:"path,omitempty"`
}

// CLIMeta holds the CLI mapping for a method's input struct.
// Generated at compile time by ktsp-schemagen.
type CLIMeta struct {
	Args  []CLIArg  `json:"args,omitempty"`
	Flags []CLIFlag `json:"flags,omitempty"`
}

// MethodInfo describes a method available on a slot.
//
// InputSchema and OutputSchema are typed as `any` (rather than
// `json.RawMessage`) so both JSON and YAML serialization produce structured
// output. `json.RawMessage` is `[]byte` underneath, which yaml.v3 encodes as
// a sequence of integer byte values — illegible and unusable on reload.
// Storing the decoded tree lets both encoders walk it naturally.
type MethodInfo struct {
	Name         string     `json:"name"              yaml:"name"`
	Description  string     `json:"description,omitempty" yaml:"description,omitempty"`
	InputSchema  any        `json:"inputSchema,omitempty"  yaml:"inputSchema,omitempty"`
	OutputSchema any        `json:"outputSchema,omitempty" yaml:"outputSchema,omitempty"`
	Tags         MethodTags `json:"tags,omitempty"     yaml:"tags,omitempty"`
	// TimeoutS is the per-method request timeout in seconds advertised to
	// callers (0 = transport default). See meta.MethodDef.TimeoutS.
	TimeoutS int `json:"timeoutS,omitempty" yaml:"timeoutS,omitempty"`
}

// SlotTypeDep declares a slot type that this slot needs from the runtime.
type SlotTypeDep struct {
	Type     SlotType `json:"type"`
	Required bool     `json:"required"`
}

// SlotInfo describes a logical endpoint within a
// A slot provides a capability (SlotType) and may consume others (Requires).
type SlotInfo struct {
	Name        string        `json:"name"                  yaml:"name"`
	Description string        `json:"description,omitempty" yaml:"description,omitempty"`
	SlotType    SlotType      `json:"slot_type,omitempty"   yaml:"slot_type,omitempty"`
	Requires    []SlotTypeDep `json:"requires,omitempty"    yaml:"requires,omitempty"`
	Tags        SlotTags      `json:"tags,omitempty"        yaml:"tags,omitempty"`
	Methods     []MethodInfo  `json:"methods,omitempty"     yaml:"methods,omitempty"`
}

// SlotTypeContract describes the method contract for a SlotType.
// Auto-generated from the methods registered on slots of this type.
type SlotTypeContract struct {
	Type        SlotType     `json:"type"                  yaml:"type"`
	Version     string       `json:"version"               yaml:"version"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	Methods     []MethodInfo `json:"methods"               yaml:"methods"`
}

// ContainerInfo describes a container and all its slots.
type ContainerInfo struct {
	Name        string `json:"name"                    yaml:"name"`
	Description string `json:"description,omitempty"   yaml:"description,omitempty"`
	Version     string `json:"version,omitempty"       yaml:"version,omitempty"`
	// ProtocolVersion is the KTSP wire-protocol major this container speaks
	// (KTSPProtocolVersion). The hub negotiates it at first contact. Absent/0 on
	// the wire = a pre-versioning sidecar → treated as the baseline major.
	ProtocolVersion int                `json:"protocol_version,omitempty" yaml:"protocol_version,omitempty"`
	SlotTypes       []SlotTypeContract `json:"slot_types,omitempty"    yaml:"slot_types,omitempty"`
	Slots           []SlotInfo         `json:"slots,omitempty"         yaml:"slots,omitempty"`
	Tags            []string           `json:"tags,omitempty"          yaml:"tags,omitempty"`
}
