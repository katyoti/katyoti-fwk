// SPDX-License-Identifier: Apache-2.0

// Package publish defines types for the service publishing framework.
// PublishedService describes a service and its capabilities (KTSP, HTTP, etc.).
// Protocol adapters in kts/local/ consume these to generate wiring.
//
// This package is in core/ so both appcore (publish functions) and kts/local/
// (protocol adapters) can import it without circular dependencies.
package publish

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// PublishedService declares a service and its publishing capabilities.
// Contributed to the PublishedServices EM point; protocol adapters
// (AutoKTSPContainer, AutoHTTPServer) read these to generate wiring.
type PublishedService struct {
	// Identity
	Name        string // derived from TypeName[TService]()
	Description string
	Version     string

	// Service type (for reflection-based dispatch)
	ServiceType reflect.Type

	// Infrastructure needs
	ConfigFiles []string

	// suppressOwnConfigFile drops the derived config file, set by
	// WithoutConfigFile. Applied after every option has run, so the order of
	// the option list does not decide the outcome.
	suppressOwnConfigFile bool

	// ownConfigFile is the file derived from the service type name — the one
	// the service manager actually loads from. Kept apart from ConfigFiles so
	// WithoutConfigFile can drop exactly it and not a caller's addition.
	ownConfigFile string

	// ConfigShape is the JSON Schema of THIS service's config type, derived
	// automatically at publication.
	//
	// It is what lets a caller render a FORM for a config file rather than
	// only a health report: without a shape there are no fields, so the best a
	// consumer could do was say what was missing and not let anyone fix it.
	//
	// It is NOT keyed by file name, because the two names are derived from
	// different things: a config FILE is named after the service
	// (`GocodeService.yaml`), while the config TYPE it decodes into may be
	// called anything (`core.Config`). Keying by the type's name linked
	// nothing for exactly those services. The file is resolved instead from
	// what this service declares.
	//
	// Populated only for a service published WITH a typed config. A service
	// whose config is untyped has none, and a consumer falls back to the
	// health report — an absent shape is a normal state, not a failure, and
	// rendering an empty form would be worse than rendering none.
	ConfigShape json.RawMessage

	ConfigDirs []string
	Tools      []ToolDef
	APIKeys    []APIKeyDef
	Auth       []AuthDef

	// Definition describes the service interface (from api/ package).
	Definition *meta.ServiceDef

	// Capabilities — what protocol adapters should generate
	Capabilities []Capability

	// Local marks this PublishedService as binary-private — its
	// auto-generated `Type: "service"` Contribution is emitted with
	// `Provider == contribapi.LocalProvider` instead of the binary's
	// self-provider name, and its method I/O types are NOT auto-published
	// as data-type Contributions (those are private to the binary).
	//
	// In-process resolution is unaffected: callers in the SAME binary still
	// resolve it via `application.UseService[T](ctx)`.
	//
	// Set by `WithLocal()`. Composes orthogonally with `WithKTSPContainer`
	// at the model level (a service CAN be both — meaning "exposed on the
	// bus AND advertised as local-scope"), but in practice the two are
	// mutually exclusive: most local services declare ONLY `WithLocal()`
	// and skip every wire-exposing option.
	//
	// See: contribution-meta-model.md §4.3 (Phase E addendum).
	Local bool
}

// FindCapability returns the first capability matching the given name, or nil.
func (ps *PublishedService) FindCapability(name string) Capability {
	for _, c := range ps.Capabilities {
		if c.CapabilityName() == name {
			return c
		}
	}
	return nil
}

// HasCapability returns true if the service has a capability with the given name.
func (ps *PublishedService) HasCapability(name string) bool {
	return ps.FindCapability(name) != nil
}

// --- Capability interface + types ---

// Capability marks what a PublishedService can do.
type Capability interface {
	CapabilityName() string
}

// KTSPContainerCap — service exposed via KTSP container.
type KTSPContainerCap struct {
	SlotName    string
	SlotDesc    string
	SlotVersion string
	Methods     map[string]meta.MethodDef
	// CLIGroups carries the ServiceDef's noun-verb command groups. AutoKTSPContainer
	// serializes them into the slot's `cli.groups` tag for the CLI generator.
	CLIGroups []meta.CLIGroup
	// SlotTags are open-ended key/value metadata applied to the generated KTSP
	// slot. AutoKTSPContainer merges these into the slot's tag map alongside
	// any tags it sets itself (e.g. cli.flat). Use this to opt a service out of
	// auto-CLI generation (`cli=false`) while keeping it reachable via KTSP/HTTP.
	SlotTags map[string]string
}

func (c KTSPContainerCap) CapabilityName() string { return "ktsp-container" }

// KTSPNamedInstancesCap — service exposes additional named slot instances
// resolved at AppInit time by a provider function.
//
// Where KTSPContainerCap binds exactly one slot (the DefaultInstance bucket)
// to the service registered in ServiceManager, KTSPNamedInstancesCap is the
// fan-out form: the provider returns a `name → impl` map, and AutoKTSPContainer
// registers one *ktsp.Slot per entry under that name via Endpoint.AddSlot(slot, instance).
//
// Each instance becomes addressable on the wire as
//
//	<provider>:<slot-type>(<name>)/<method>
//
// using the same Method shapes / JSON schemas as the slot type. The two caps
// compose: a service can declare WithKTSPContainer (binds "default" → SM service)
// AND WithKTSPNamedInstances (binds "books-pg", "leanix-prod", … → provider impls).
//
// Used by the CRUD framework to expose one logical CRUDService slot type across
// many named connections without re-publishing per connection.
type KTSPNamedInstancesCap struct {
	SlotName    string
	SlotDesc    string
	SlotVersion string
	Methods     map[string]meta.MethodDef
	SlotTags    map[string]string

	// Provider returns name → service-impl. Called once at AppInit time after
	// every other Extension has had a chance to register its services, so the
	// provider can safely close over (and read from) the ServiceManager. A
	// non-nil error aborts AutoKTSPContainer startup with that error wrapped.
	Provider func(app *application.Application) (map[string]any, error)
}

func (c KTSPNamedInstancesCap) CapabilityName() string { return "ktsp-named-instances" }

// HTTPServerCap — service exposed via HTTP server.
type HTTPServerCap struct {
	HealthEndpoint bool
	DefaultHost    string
	DefaultPort    int
}

func (c HTTPServerCap) CapabilityName() string { return "http-server" }

// AgentPluginCap — service contributes agent plugin strategy.
type AgentPluginCap struct {
	StrategyName string
	Label        string
}

func (c AgentPluginCap) CapabilityName() string { return "agent-plugin" }

// --- Infrastructure types ---

// ToolDef describes an external tool dependency.
type ToolDef struct {
	Name         string   // display name (e.g. "Pandoc")
	ConfigKey    string   // settings.yaml key (e.g. "pandoc_path")
	Commands     []string // executable names to LookPath
	InstallSkill string   // canonical skill URI for install instructions (e.g. "ktw-docs:/skill/install-pandoc")
	VersionFlag  string   // flag to get version (e.g. "--version")
	// Version is the version the sidecar is validated against (e.g. "3.11" —
	// the one its InstallSkill installs). When set, a found version that does
	// not start with it is reported "outdated": the tool runs but its output
	// may not be what the sidecar expects. Requires VersionFlag.
	Version string
	// Optional marks a program the sidecar runs without: a missing optional
	// program is a warning in the config check, and an installer offers it
	// without preselecting it. The zero value is a required program.
	Optional bool
	// Recipes are the structured install steps per platform (install_recipe.go),
	// alternatives in order of preference for each platform; InstallSkill stays
	// the prose fallback. Validated at boot (Validate): a declaration an engine
	// could not run is refused.
	Recipes []InstallRecipe
	// PythonPackage names the Python package the program is (its pip recipes'
	// package): it is checked as that package of the Python pip installs with
	// (CheckPythonPackage), never as a command on PATH. Takes Commands' and
	// Check's place.
	PythonPackage string
	Check         any // custom check function (typed by consumer)
}

// APIKeyDef describes an API key used by the application.
type APIKeyDef struct {
	EnvName     string // e.g. "ANTHROPIC_API_KEY"
	Label       string // e.g. "ElevenLabs API key"
	Required    bool   // true = required for core functionality
	Description string // shown in check report
}

// AuthDef declares an authentication / login flow the sidecar owns. Unlike an
// API key (a static env var), auth is an interactive login. Declaring it makes
// the login DISCOVERABLE, so a generic configurator (katyoti) can route to it
// via `config check` (category "auth") instead of provider-specific branches.
type AuthDef struct {
	Name        string // auth identity, e.g. "google"
	Label       string // e.g. "Google Workspace login"
	Description string
	// LoginHint is how to authenticate — a command or skill URI the operator
	// (or the configure engine) runs, e.g. "ktw-example auth login".
	LoginHint string
	// Check optionally reports whether authentication is already in place. If
	// nil, the check reports the auth state as unknown (and points at LoginHint).
	Check func(ctx context.Context) (bool, error)
}

// --- Publish options ---

type PublishOption func(*PublishedService)

func WithDescription(desc string) PublishOption {
	return func(ps *PublishedService) { ps.Description = desc }
}

func WithServiceDef(def meta.ServiceDef) PublishOption {
	return func(ps *PublishedService) {
		ps.Definition = &def
		if ps.Description == "" {
			ps.Description = def.Description()
		}
	}
}

func WithVersion(version string) PublishOption {
	return func(ps *PublishedService) { ps.Version = version }
}

// WithKTSPSlotTags attaches additional slot tags to the KTSPContainerCap.
// Must be applied AFTER WithKTSPContainer; merges into existing tags, last
// writer wins. Use TagSlotCLI from internal/kts/protocol/ktsp (key "cli")
// with value "false" to suppress auto-CLI generation for this service.
func WithKTSPSlotTags(tags map[string]string) PublishOption {
	return func(ps *PublishedService) {
		for i := range ps.Capabilities {
			cap, ok := ps.Capabilities[i].(KTSPContainerCap)
			if !ok {
				continue
			}
			if cap.SlotTags == nil {
				cap.SlotTags = make(map[string]string, len(tags))
			}
			for k, v := range tags {
				cap.SlotTags[k] = v
			}
			ps.Capabilities[i] = cap
			return
		}
	}
}

func WithKTSPContainer(overrides ...string) PublishOption {
	return func(ps *PublishedService) {
		cap := KTSPContainerCap{}
		if ps.Definition != nil {
			// SlotName must be in the kebab wire form: it's used as the
			// NATS subject token in `messagebus.Subscribe`, which enforces
			// [a-z0-9-]{1,63}. ResolvedBusName returns the explicit
			// ServiceDef.BusName when set, falling back to a mechanical
			// kebab of the PascalCase Name (e.g., "CatalogService" →
			// "catalog-service", "LLMProfiles" → "llm-profiles"). This
			// keeps the in-process Endpoint slot key consistent with
			// what arrives over the wire — the Sender's URI parser
			// also kebabs the Type token at parse time.
			cap.SlotName = ps.Definition.ResolvedBusName()
			cap.SlotDesc = ps.Definition.Description()
			cap.SlotVersion = ps.Definition.Version
			cap.Methods = ps.Definition.Methods
			cap.CLIGroups = ps.Definition.CLIGroups
		}
		if len(overrides) > 0 && overrides[0] != "" {
			cap.SlotName = overrides[0]
		}
		if len(overrides) > 1 && overrides[1] != "" {
			cap.SlotDesc = overrides[1]
		}
		if len(overrides) > 2 && overrides[2] != "" {
			cap.SlotVersion = overrides[2]
		}
		ps.Capabilities = append(ps.Capabilities, cap)
	}
}

// WithKTSPNamedInstances declares additional named slot instances for the
// service. The provider is invoked once at AppInit time (after all services
// are registered) and returns a `name → impl` map; AutoKTSPContainer registers
// each entry as its own *ktsp.Slot under the same slot type via
// Endpoint.AddSlot(slot, instance). Wire addressing per instance:
//
//	<provider>:<slot-type>(<name>)/<method>
//
// Compose with WithKTSPContainer when the SM service should ALSO answer under
// the DefaultInstance bucket; use alone when every binding is named (e.g. the
// CRUD framework, where each connection is a strategy-built instance and there
// is no meaningful "default" service in SM).
//
// SlotName, SlotDesc, SlotVersion, Methods default from ps.Definition so a
// service typically declares this option with no extra arguments.
func WithKTSPNamedInstances(provider func(app *application.Application) (map[string]any, error)) PublishOption {
	return func(ps *PublishedService) {
		cap := KTSPNamedInstancesCap{Provider: provider}
		if ps.Definition != nil {
			// Same wire-name resolution as WithKTSPContainer — kebab form so
			// the messagebus subject token validates.
			cap.SlotName = ps.Definition.ResolvedBusName()
			cap.SlotDesc = ps.Definition.Description()
			cap.SlotVersion = ps.Definition.Version
			cap.Methods = ps.Definition.Methods
		}
		ps.Capabilities = append(ps.Capabilities, cap)
	}
}

// WithKTSPNamedInstanceTags attaches slot tags to the KTSPNamedInstancesCap
// after WithKTSPNamedInstances has been applied. Behaves like
// WithKTSPSlotTags for the KTSPContainerCap.
func WithKTSPNamedInstanceTags(tags map[string]string) PublishOption {
	return func(ps *PublishedService) {
		for i := range ps.Capabilities {
			cap, ok := ps.Capabilities[i].(KTSPNamedInstancesCap)
			if !ok {
				continue
			}
			if cap.SlotTags == nil {
				cap.SlotTags = make(map[string]string, len(tags))
			}
			for k, v := range tags {
				cap.SlotTags[k] = v
			}
			ps.Capabilities[i] = cap
			return
		}
	}
}

func WithHTTPServer(defaultPort ...int) PublishOption {
	return func(ps *PublishedService) {
		cap := HTTPServerCap{HealthEndpoint: true, DefaultHost: "127.0.0.1", DefaultPort: 8080}
		if len(defaultPort) > 0 {
			cap.DefaultPort = defaultPort[0]
		}
		ps.Capabilities = append(ps.Capabilities, cap)
	}
}

func WithAgentPlugin(strategyName, label string) PublishOption {
	return func(ps *PublishedService) {
		ps.Capabilities = append(ps.Capabilities, AgentPluginCap{
			StrategyName: strategyName, Label: label,
		})
	}
}

// WithoutConfigFile declines the automatic advertisement of this service's own
// config file.
//
// For the case where a binary publishes a service but does not honour part of
// its config. A plain sidecar hosts SkillManager for its embedded skills and
// ignores the `extra_dirs` knob — advertising the file there would make
// `sc config check` report a missing, default-less config for something the
// binary never reads.
//
// Declining is the exception and must be argued at the call site. The default
// is to advertise, because the service DOES load the file and an operator who
// cannot see it has no way to learn that.
func WithoutConfigFile() PublishOption {
	return func(ps *PublishedService) { ps.suppressOwnConfigFile = true }
}

// WithConfigFiles advertises additional config files.
//
// A service's OWN file is advertised by PublishService, derived from the
// service type name — the same name the service manager loads from. Use this
// only for a FURTHER file the service reads.
//
// Duplicates are dropped rather than appended: a declaration that restates the
// derived name is redundant, not an error, and listing the same file twice
// would show it twice in `sc config check`.
func WithConfigFiles(files ...string) PublishOption {
	return func(ps *PublishedService) {
		for _, f := range files {
			if slices.Contains(ps.ConfigFiles, f) {
				continue
			}
			ps.ConfigFiles = append(ps.ConfigFiles, f)
		}
	}
}

func WithConfigDirs(dirs ...string) PublishOption {
	return func(ps *PublishedService) { ps.ConfigDirs = append(ps.ConfigDirs, dirs...) }
}

func WithTools(tools ...ToolDef) PublishOption {
	return func(ps *PublishedService) { ps.Tools = append(ps.Tools, tools...) }
}

func WithAPIKeys(keys ...APIKeyDef) PublishOption {
	return func(ps *PublishedService) { ps.APIKeys = append(ps.APIKeys, keys...) }
}

// WithAuth declares one or more login flows the sidecar owns (see AuthDef).
// Surfaced by `config check` under category "auth" so a generic configurator
// can drive login without provider-specific code.
func WithAuth(auths ...AuthDef) PublishOption {
	return func(ps *PublishedService) { ps.Auth = append(ps.Auth, auths...) }
}

// WithLocal marks the service as binary-private (Phase E §4.3). The
// auto-emitted `Type: "service"` Contribution gets `Provider: "local"`
// instead of the binary's self-provider name, and the method I/O
// types are NOT auto-published as `Type: "data-type"` Contributions —
// they stay private to the binary.
//
// In-process resolution is unaffected. Use for services that:
//   - Are consumed only by other in-process strategies (e.g., SkillManager
//     called by the agent loop and the local CLI; the task runner in
//     ktw-agents),
//   - Have no useful semantics over the wire (debug / introspection
//     helpers),
//   - Carry data shapes that the binary intends to keep private.
//
// Typically you call `WithLocal()` INSTEAD of `WithKTSPContainer()` —
// the two compose at the model level but in practice are mutually
// exclusive (a local service is one that's NOT bus-exposed).
func WithLocal() PublishOption {
	return func(ps *PublishedService) { ps.Local = true }
}

// --- EM point ---

// NewPublishedServicesPoint makes the EM list point for published services —
// a FRESH one per application: a point is state, and one instance shared by
// every application of a process leaks the first one's services into the next.
func NewPublishedServicesPoint() em.ExtensionPoint[PublishedService] {
	return em.NewListPoint[PublishedService]()
}

// WithConfigShape records the JSON Schema of a config file's Go struct.
//
// Applied automatically by PublishService, which is the only place that knows
// both the file's name and the type it decodes into. Deriving it there is what
// makes this automatic for every typed config rather than a per-sidecar opt-in
// list — an opt-in list would mean the sidecars nobody remembered to add stay
// unconfigurable, which is the same gap in a new place.
func WithConfigShape(schema json.RawMessage) PublishOption {
	return func(ps *PublishedService) {
		if len(schema) > 0 {
			ps.ConfigShape = schema
		}
	}
}
