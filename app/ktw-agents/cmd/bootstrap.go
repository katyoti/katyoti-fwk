// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	profileagentplugin "github.com/katyoti/katyoti-fwk/internal/extensions/profile/agentplugin"
	profilelocal "github.com/katyoti/katyoti-fwk/internal/extensions/profile/local"
	skillagentplugin "github.com/katyoti/katyoti-fwk/internal/extensions/skills/agentplugin"
	taskcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib"

	cwmembed "github.com/katyoti/katyoti-fwk/app/ktw-agents/embed/cwm"
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	busclientbus "github.com/katyoti/katyoti-fwk/internal/appcore/busclient/bus"
	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	agentcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/agent/contrib"
	agentlocal "github.com/katyoti/katyoti-fwk/internal/extensions/agent/local"
	keepalive "github.com/katyoti/katyoti-fwk/internal/extensions/agent/local/keepalive"
	codelocal "github.com/katyoti/katyoti-fwk/internal/extensions/code/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmlocal "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/local"
	gocodelocal "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/local"
	llmlocal "github.com/katyoti/katyoti-fwk/internal/extensions/llm/local"
	methodcommands "github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/commands"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/cli"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	terminalroom "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"

	// docsplugin removed — docs tools auto-exposed via KTSP sidecar discovery
	"github.com/katyoti/katyoti-fwk/internal/extensions/humangate"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/mcp"

	// notifyplugin removed — notify tools auto-exposed via KTSP sidecar discovery
	actionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/actions/local"
	commentscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/comments/contrib"
	commentslocal "github.com/katyoti/katyoti-fwk/internal/extensions/comments/local"
	commslocal "github.com/katyoti/katyoti-fwk/internal/extensions/comms/local"
	crudlocal "github.com/katyoti/katyoti-fwk/internal/extensions/crud/local"
	cwmcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/contrib"
	displaybridgelocal "github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/local"
	eventslocal "github.com/katyoti/katyoti-fwk/internal/extensions/events/local"
	fileoverviewlocal "github.com/katyoti/katyoti-fwk/internal/extensions/fileoverview/local"
	gatescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/gates/contrib"
	gateslocal "github.com/katyoti/katyoti-fwk/internal/extensions/gates/local"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	inboxlocal "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/local"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/ledger"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	manifestuniverse "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/universe"
	occontrib "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/contrib"
	oclocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/local"
	panecontrib "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/contrib"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestration"
	backupjob "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/backupjob"
	panelservercontrib "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/contrib"
	panelserverremote "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/remote"
	personaslocal "github.com/katyoti/katyoti-fwk/internal/extensions/personas/local"
	planificationcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/planification/contrib"
	planificationlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	projectcontributionscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/contrib"
	projectcontributionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/local"
	publicationscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/publications/contrib"
	publicationslocal "github.com/katyoti/katyoti-fwk/internal/extensions/publications/local"
	reviewscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/contrib"
	reviewslocal "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/local"
	runnerlocal "github.com/katyoti/katyoti-fwk/internal/extensions/runner/local"
	schedulerlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	schemalocal "github.com/katyoti/katyoti-fwk/internal/extensions/schema/local"
	scriptslocal "github.com/katyoti/katyoti-fwk/internal/extensions/scripts/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/simulator"
	skillscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/skills/contrib"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	ktspengine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	tasklocal "github.com/katyoti/katyoti-fwk/internal/extensions/task/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/template"
	terminallocal "github.com/katyoti/katyoti-fwk/internal/extensions/terminal/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/filestate"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/ask_user"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/ask_user_data"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/bash"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/complete_ui_component"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/create_document"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/deck_export"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/diagram_emit"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/display_ui_component"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/file_read"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/file_write"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/patch_ui_component"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/records"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/rest_api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/session_read"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/toolproxy"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/web_fetch"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/web_search"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/wikipedia"
	universeviewlocal "github.com/katyoti/katyoti-fwk/internal/extensions/universeview/local"
	verificationcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/verification/contrib"
	verificationlocal "github.com/katyoti/katyoti-fwk/internal/extensions/verification/local"
	weblocal "github.com/katyoti/katyoti-fwk/internal/extensions/web/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/workspace"
	"github.com/spf13/cobra"
)

// runtimeContext holds shared resources for extension initialization.
type runtimeContext struct {
	Config     loader.Loader
	HomeDir    string
	EmbeddedFS fs.FS
	AddCommand func(cmd *cobra.Command)
}

// pluginsYAMLExt contributes the Plugins.yaml ConfigProvider to the plugin domain.
// Loads PluginsConfig from the config system at Contribute time — no pre-loading needed.
// Registered after leaf extensions so strategies exist when AppInit runs.
var pluginsYAMLExt application.Extension = func(init *application.ApplicationInit) {
	init.AddContribute("plugins.yaml", "", func(app *application.Application) error {
		pluginsCfg, _ := application.AppUseConfig[agentapi.PluginsConfig](app)
		return agentlocal.ContributeProvider(app, agentlocal.NewPluginsYAMLProvider(pluginsCfg.Plugins, nil))
	})
}

// buildApp creates the Application with all built-in extensions.
// Call app.Run() after to execute the 4-phase bootstrap.
// privateExtensions are the extensions of private modules the development
// build hosts, appended by private.go; the open-source build (tag oss) has none.
var privateExtensions []application.Extension

func buildApp(ctx runtimeContext) *application.Application {
	exts := []application.Extension{
		// Infrastructure — owns extension points + core infra manifest (must be first)
		appcore.AppCoreExt,
		CoreInfraExt,
		// Self-provider stamp — ktw-agents has a custom buildApp that
		// doesn't go through `appcore.NewAppExt(cfg)`, so we wire the
		// equivalent SetProvider call here. SidecarExt's contribution
		// producers read `app.Provider()` at AppInit and fail loudly
		// without this.
		appcore.SetProviderExt("ktw-agents"),
		// Cobra groups MUST be registered before SidecarExt + llmlocal so
		// the `sc` and `llm` named services exist when those bootstrap.
		appcore.SidecarGroupsExt(),
		appcore.SidecarExt(ctx.EmbeddedFS), // sidecar contract: config, llm, sidecar commands
		llmlocal.Extension(),               // LLM-facing surface (info, manifest, tools, resources)
		// Manifest infrastructure: ContributionProvidersPoint + ResourceService +
		// `resources list/get` + the three config-as-resources providers
		// (config-file / config-program / env). Lets ktw-agents participate
		// in the catalog/refresh ecosystem like every other sidecar.
		manifestlocal.Extension(),
		manifestlocal.ContributeConfigResources(ctx.EmbeddedFS),
		agentlocal.NewPluginDomainExt(),
		agentlocal.ProvidersExtension,
		agentlocal.ProfilesExtension,
		ktspengine.MachineDomain.Extension(), // Machine domain — ONE domain for all tasks

		// Agent machine strategy (contributes "agent-loop" + "task" to MachineDomain)
		agentlocal.MachineStrategyExtension,

		// Services (registered before plugins that depend on them)
		//
		// docs + schema service publishers are NOT bundled here — they are hosted
		// by their dedicated sidecars (ktw-docs / ktw-hub) per
		// `dedupe-pack-services-from-agents` (Phase 1). ktw-agents previously
		// re-published DocService + the legacy SchemaService in-process, which
		// competed with the sidecars over NATS and pulled in irrelevant
		// `config check` probes (pandoc/typst/drawio). diagram-emit still works
		// (it calls the docsrender package directly, not the service); agent-side
		// schema validation is served by ktw-hub; displaybridge bridges to
		// ktw-display. code + gocode services remain bundled pending the
		// LanguagePack-over-bus rewiring (Phase 2).
		displaybridgelocal.Extension(),      // Phase-1 stub bridge to ktw-display TS sidecar; lazy-spawns bun on first slot call
		actionslocal.Extension(),            // ActionService dispatcher: tasks / scripts / KTSP method calls via one URI surface (cwm-v2-providers US-15)
		actionslocal.SurfaceExtension(),     // the action surface (ktw-agents:action-surface/List): what an object, a type's list or the universe offers
		eventslocal.Extension(),             // EventService facade over MessageBus: EventURI subscribe + publish (cwm-v2-providers US-15)
		schemalocal.RemoteExtension(),       // bus-backed SchemaService CLIENT (real service hosted by ktw-hub): consumers resolve it via application.UseService[SchemaService], bus round-trip encapsulated
		panelserverremote.RemoteExtension(), // bus-backed PanelService CLIENT (the panel server runs in ktw-hub): a workflow ask reads its panel's log after a restart
		busclientbus.Extension(),            // the bus-backed Sender the generated/remote clients call through

		// Contexia Workspace Manager — publishes 38 typed methods +
		// 26 cwm-* agent tools. WorkspaceRoot is resolved lazily on
		// first service-use via the discovery chain
		// (--workspace-dir > KTW_WORKSPACE_DIR > walk-up .katyoti >
		// first-run <CWD>/.katyoti).
		cwmlocal.ExtensionWith(cwmlocal.Options{
			WorkspaceRootProvider: cwmWorkspaceRootProvider,
			TypesFS:               cwmembed.FS,
			ServedElsewhere:       clientMode, // a client of its running universe opens no workspace
		}),
		cwmlocal.AgentPluginExtension(),
		personaslocal.Extension(),                 // PersonaService: resolve / list / spawn catalog personas (gocode's module orchestrate spawns through it)
		cwmcontrib.CorePack(""),                   // the object-core data-type: what every object is made of, declared by CWM
		inboxcontrib.Pack(""),                     // the inbox and approval-request types, declared by the inbox extension
		inboxcontrib.DeclareKinds(""),             // the ask-kind kind: a kind of question a human task asks
		inboxcontrib.OwnAskKinds(""),              // the nine built-in kinds of questions (approve, decide, test-by-hand, form, provide, choose, permission, unblock, accept)
		panelservercontrib.Pack(""),               // the generic panel types: steps, log, question and chat - ktw-agents:/panel-type/<name>
		journalcontrib.Pack(""),                   // the journal-day type, declared by the journal extension
		journallocal.Extension(),                  // JournalService (ktw-agents:journal/<Verb>) + the CWM observer that records every object write, in the process holding the workspace
		inboxlocal.Extension(),                    // InboxService (ktw-agents:inbox/<Verb>): the inbox's one writer — the agent's inbox channel and the GUI Inbox tab go through it
		inboxlocal.ChecksExtension(),              // the guide's inbox checks (ktw-agents:inbox-checks/<Verb>): read-only, a recipe's check
		inboxlocal.ApprovalExtension(),            // the one-tap phone approval (ktw-agents:approval/<Verb>) and its page on 127.0.0.1, published to the tailnet with tailscale serve
		inboxlocal.HumanTaskExtension(),           // the human task: the human-task machine strategy and the collector of its person to-dos
		inboxlocal.WaitingExtension(),             // « À faire », the one place: ktw-agents:waiting/<Verb>, and the point the askers contribute their questions to
		profilelocal.PasskeyServiceExtension(),    // the person's passkey (ktw-agents:passkey/<Verb>): the approval page's ceremonies and the check the guards of the person's acts ask
		profilelocal.PermissionServiceExtension(), // the permission codes (ktw-agents:permission/<Verb>): every act's code, declared or derived, and the check - allowed until grants exist
		planificationcontrib.Pack(""),             // the request type, its workflow, the depends-on edge and the ready-gate / bootstrap hooks, declared by the planification extension
		schedulerlocal.Extension(),                // the scheduler (ktw-agents:todos/<Verb>): each module's to-dos as files, pulled and reported by workers inside and outside
		backupjob.Extension(),                     // the universe backup job (one run an hour while serving), and « Name the backup targets » in « À faire » while none is named
		eventslocal.ReactionsExtension(),          // reactions (ktw-agents:reactions/<Verb>): a CWM async subscription matched by a write becomes a to-do of its module, run under its cause, its events released on success
		eventslocal.SourcesExtension(),            // events from outside the objects (ktw-agents:event-sources/<Verb>): a git HEAD (code.changed), timers (the timed capability, delayed events), a call from outside - raised on their object
		planificationlocal.Extension(),            // RequestService (ktw-agents:requests/<Verb>) + the CWM observer that wakes requests parked on their dependencies
		planificationlocal.AgentPluginExtension(), // the file-request chat tool
		schedulerlocal.AgentPluginExtension(),     // the todos plugin: the worker contract as tools, for the work-todo task
		occontrib.Pack(""),                        // the context capability: a type's `context:` section (inheritance, briefs), and the agent-context kind
		oclocal.Extension(),                       // ObjectContextService (ktw-agents:object-context/<Verb>, ObjectContextService.yaml): what a chat on an object receives, and inheritance
		oclocal.AgentPluginExtension(),            // the object-context plugin, attached by the agent factory to every run with a target object
		fileoverviewlocal.Extension(),             // FileOverviewService (ktw-agents:file-overview/read): an object's overview file, the file-overview panel's source
		commentscontrib.Pack(""),                  // the comments capability: a thread on every object, kept with it
		commentslocal.Extension(),                 // CommentsService (ktw-agents:comments/<Verb>): say something about an object, answer, read
		commentslocal.AgentPluginExtension(),      // the comment-list / comment-add chat tools (plugin `comments`)
		gatescontrib.Pack(""),                     // the gates capability: the confirmations group and gate() in guards
		gateslocal.Extension(),                    // GatesService (ktw-agents:gates/<Verb>): named gates — checklists and a person's confirmations
		gateslocal.AgentPluginExtension(),         // the gate-check chat tool (plugin `gates`)
		panecontrib.Pack(""),                      // the pane capability: a type's `pane:` section (view, badges), and the object-panel kind
		panelocal.Extension(),                     // ObjectPaneService (ktw-agents:object-pane/<Verb>): the detail pane in one round trip, and where an object may move, judged by CWM's dry-run
		panelocal.AgentPluginExtension(),          // the object-transition-options chat tool (plugin `object-pane`)
		reviewscontrib.Pack(""),                   // the review and finding types, their workflows, the reviewable capability and the Reviews / Review panels
		projectcontributionscontrib.Pack(""),      // the contributions capability: where a project keeps what it contributes to its universe; the Contributions panel
		projectcontributionslocal.Extension(),     // ProjectContributionsService (ktw-agents:project-contributions/<Verb>): the source of the Contributions panel
		reviewslocal.Extension(),                  // ReviewsService (ktw-agents:reviews/<Verb>): the sources of the Reviews and Review panels
		universeviewlocal.Extension(),             // UniverseViewService (ktw-agents:universe-view/<Verb>): what the universe's GUI shows at a display level, from its tags
		publicationscontrib.Pack(""),              // the publisher API's data types: publication-connector, document-tree
		publicationscontrib.DeclareKinds(""),      // the publication-connector kind: a publisher, its instances' settings and its publish method
		publicationslocal.Extension(),             // PublishersService (ktw-agents:publishers/<Verb>): the contributed publishers, a document tree published through one
		sourcescontrib.Pack(""),                   // the source-document and information-source types, the derived-from and feeds edges
		sourcescontrib.DeclareKinds(""),           // the source-connector kind: a kind of place an information source reads from
		sourcescontrib.OwnConnectors(""),          // the local-folder connector (sources/folder-sync)
		sourceslocal.Extension(),                  // SourcesService (ktw-agents:sources/<Verb>): read an outside document again, report what drifted
		sourceslocal.AgentPluginExtension(),       // the source-reanchor chat tool (plugin `sources`)
		verificationcontrib.Pack(""),              // the verification capability: verify() and lint() in guards and gate checks
		verificationlocal.Extension(),             // VerificationService (ktw-agents:verification/<Verb>): reads CWM's verify and lint points
		verificationlocal.AgentPluginExtension(),  // the object-verify / object-lint chat tools (plugin `verification`)

		// chat-loop keep-alive: turns the agent loop's "no tool call →
		// run ends" default into "no tool call → loop once more with a
		// nudge to call ask-user". Opt-in via `plugins: [chat-loop-keepalive]`
		// in TASK.md (the chat task bundles it; one-shot tasks omit it
		// and keep their natural termination semantics).
		keepalive.AgentPluginExtension(),
		// method-core assistant view — "where are we / what's next" (read-only,
		// in-process via CWMService), through the method assistant's one gather.
		appcore.CommandExt(methodcommands.NewMethodOverviewCommand()),
		// method-core demo step — run a node's use-case probes, write status_cache.
		appcore.CommandExt(codelocal.NewMethodDemoCommand()),
		// method-core act step — run the top What's-Next action.
		appcore.CommandExt(methodcommands.NewMethodActCommand()),

		// code + gocode SERVICES are NOT bundled here — they are hosted by their
		// dedicated sidecars (ktw-code / ktw-gocode) per
		// `dedupe-pack-services-from-agents` (Phase 2, decision B: full clean
		// move). ktw-agents previously re-published CodeService + GocodeService +
		// GoToolService + the in-process LanguagePack, competing with the sidecars
		// over NATS. Now: the agent reaches the code-methodology verbs over the
		// bus (ActionService URI dispatch → ktw-code); ktw-code resolves
		// LanguagePack over the bus to the ktw-gocode peer (BusLanguagePack);
		// CWM types + pack content come from the aggregate catalog (already
		// sidecar-owned). Compose CWM types likewise come from ktw-compose.
		//
		// gocode's "Suggested next step" agent plugin STAYS in-process — it's a Go
		// AgentPlugin the loop loads in-process by nature. Opt-in via
		// plugins:[gocode-next-step] in TASK.md frontmatter.
		gocodelocal.AgentPluginExtension(),

		// Plugin + task extensions
		template.Extension,
		// docs + notify plugins removed — auto-exposed via KTSP sidecar discovery
		humangate.Extension,
		simulator.Extension(),
		ledger.Extension(),
		ledger.AgentPluginExtension(),
		ask_user.Extension,
		ask_user_data.Extension,
		create_document.Extension,
		deck_export.Extension,
		display_ui_component.Extension,
		patch_ui_component.Extension,
		complete_ui_component.Extension,
		records.Extension,
		// `memory` CRUD connection — in-process record store published
		// as crud-service(memory).
		crudlocal.MemoryConnectionExtension(),
		// WithConfigFile: ktw-agents is the one binary that honors
		// SkillManager.yaml's `extra_dirs` (external skill directories), so
		// it advertises + materializes the template from its own embed.
		// Plain sidecars omit it — they ship embedded skills only.
		skillslocal.Extension(skillslocal.WithConfigFile()),
		skillagentplugin.Extension(),
		profileagentplugin.Extension(), // the person-language plugin: a run writes in the person's language (Profile.language)
		// Skills + references contribute to the manifest's ContributionProvidersPoint.
		// SkillManager filters by Type() at runtime to populate its in-memory map.
		skillscontrib.ContributeSkillResources("ktw-agents", loader.NewEmbedFSLoader(ctx.EmbeddedFS, "."), "embed/skills"),
		manifestcontrib.ContributeReferenceResources("ktw-agents", loader.NewEmbedFSLoader(ctx.EmbeddedFS, "."), "embed/references"),
		scriptslocal.Extension(),
		scriptslocal.AgentPluginExtension(), // scripts-run + scripts-list as agent tools
		scriptslocal.ContributeScriptLoader(loader.NewEmbedFSLoader(ctx.EmbeddedFS, "."), "embed/scripts", "ktw-agents"),
		// (user-layer skills/tasks loaders live in the two-layer block at
		// the end of buildApp — global + universe dirs via twoLayerLoaderDirs.)

		// Machine strategies (contribute to MachineDomain)
		// (call_method removed 2026-06-10 — the workflow `kind: call`
		// primitive + ActionService method dispatch replace it.)
		tasklocal.WorkflowExtension,
		// A long method run as a run with its panel.
		tasklocal.MethodExtension,
		// (The echo strategy test fixture is wired in private.go: its package
		// lives under modules/, which the open-source build does not carry.)

		// File-tool infrastructure (filestate cache + per-tool config)
		filestate.Extension(),
		file_read.FileReadConfigExtension,
		file_write.FileWriteConfigExtension,

		// Permission rules system: the profiles are `permission-profile`
		// contributions - the kind, its data-type and the
		// agent's base profiles (workspace-edit, plan-mode, full-access);
		// the service reads them from the catalog at the gate's first call.
		agentcontrib.DeclareKinds(""),
		agentcontrib.OwnProfiles(""),
		agentlocal.PermissionProfilesExtension,

		// Plugin-only
		workspace.Extension,
		bash.Extension,
		weblocal.Extension,
		web_fetch.Extension,
		web_search.Extension,
		wikipedia.Extension,
		rest_api.Extension,
		diagram_emit.Extension,
		session_read.Extension,
		mcp.Extension,
		tasklocal.ContextPluginExtension,
		tasklocal.TaskPluginExtension,
		toolproxy.Extension,
		orchestration.Extension,
		commslocal.Extension,        // user-communication service (Dimension 5) — the parent↔child channel agents + non-agent tasks share
		commslocal.ListsExtension(), // a parent run's holding list: its child's questions are human tasks only that run answers

		// Config providers (must be after leaf extensions that contribute strategies)
		pluginsYAMLExt,

		// TaskService (published, task sources via ContributionProvidersPoint)
		tasklocal.Extension(),
		taskcontrib.ContributeTaskResources("ktw-agents", loader.NewEmbedFSLoader(ctx.EmbeddedFS, "."), "embed/tasks"),

		// Terminal-over-KTSP service: exposes ServeTask via the core's KTSP
		// session so ktw-gui (or any KTSP client) can drive multiplexed
		// task runs against the long-lived per-universe core. Replaces
		// the legacy `terminal stdio` per-task subprocess pattern.
		terminallocal.Extension(),

		// LLM Profiles service: lives under `sc llm-profiles` (renamed
		// from the old top-level `profiles` in SN-09).
		runnerlocal.Extension(),

		// HumanGate default (CLI on stderr — TUI overrides at runtime)
		humanGateDefaultExt,

		// KTSP engine infra (EventBus, available for Subscribe/Notify)
		ktspInfraExt,

		// Terminal protocol: Room service (session management, message fan-out, bridge)
		terminalroom.Extension,

		// Sidecar plugins (discovered from Univers.yaml, contributed to plugin domain)
		sidecarPluginsExt,

		// Shared hub connection — a single process-lifetime *HubClient
		// resolved by every site that needs the universe bus (catalog
		// fetch, contributions subscriber, connmgr). Registered as an
		// ApplicationScope singleton so the cobra PersistentPostRun
		// container.Close() SIGTERMs it at exit — which only tears down
		// a hub WE spawned (ModeFindOrSpawn owns its spawn); a hub we
		// merely reused, or spawned with `--hub keep`, keeps running. Every
		// verb spawns-if-absent (hubModeFor); the exclusions reuse or no-op.
		application.DeclareSimpleService[*hubclient.HubClient](newSharedHub),

		// Expose the shared hub's MessageBus as a resolvable
		// mbapi.MessageBus service. EventService (the inbox suspend/
		// resume subscription, among others) resolves the bus via
		// application.UseService[mbapi.MessageBus](ctx); without this
		// bridge that lookup fails in-process and any EventService
		// subscribe/publish errors out. Same NATS connection the CWM
		// event publisher uses, so item-lifecycle pub/sub lines up.
		application.DeclareSimpleService[mbapi.MessageBus](func(ctx context.Context) (mbapi.MessageBus, error) {
			hc, err := application.UseService[*hubclient.HubClient](ctx)
			if err != nil {
				return nil, fmt.Errorf("resolve HubClient for MessageBus: %w", err)
			}
			return hc.MessageBus(), nil
		}),

		// Sidecar task/skill discovery (aggregates remote TaskLibrary + SkillManager)
		sidecarDiscoveryExt,

		// KTSP container — auto-builds an Endpoint from all PublishedServices
		// (TerminalService + any others). Enables `ktw-agents sc ktsp stdio` /
		// `ktw-agents ktsp describe` and lets ktw-gui's Master spawn the
		// long-lived per-universe core via KTSP.
		ktslocal.AutoKTSPContainer("ktw-agents", "ktw-agents core (per universe)", version),
		// Phase D-4: the bootstrap layer — every binary embeds the
		// same set of system contributions (contribution-type, well-
		// known data-types, system service interfaces) so it's fully
		// self-describing without the hub running. The Extension
		// registers one ContributionProvider per <type> subdirectory
		// in core/contribution/embed/ against the existing
		// ContributionProvidersPoint. See contribution-meta-model.md §4.
		// It arrives through appcore.SidecarExt above, as in every binary;
		// naming it here too registered the system providers twice
		// (ms-contribution-ext e15, ADR (modules/contributions/decisions/OD-004-which-binaries-carry-the-system-provider-and-how.cwm.md)).
		// The services producer (one `Type: "service"` contribution per
		// published WithServiceDef service) arrives through appcore.SidecarExt
		// too, its provider set by SetProviderExt above; naming it here too
		// enumerated every service twice.

		// A client of its running universe: once the slots exist, the
		// generated verbs are routed to the serving process.
		func(init *application.ApplicationInit) {
			init.AddBootstrap("hub", ".client-dispatch", func(app *application.Application) error {
				installClientDispatch(app)
				return nil
			})
		},
	}

	// Two-layer skills/tasks/sync loaders per the universe-aware ADR.
	// Global layer (~/.config/katyoti/<domain>/) provides cross-universe defaults;
	// universe layer (env.UniverseDir()/<domain>/) extends/overrides per-universe.
	// Both layers are added unconditionally; each loader silently no-ops if
	// the directory is absent.
	env := application.Env()
	for _, layer := range twoLayerLoaderDirs(env) {
		skillsDir := filepath.Join(layer.path, "skills")
		tasksDir := filepath.Join(layer.path, "tasks")
		// Declared whether the folder exists or not: the hub fingerprints
		// what ktw-agents reads, in its own universe, and re-probes it when
		// a skill or task there is added or edited — and keeps one
		// universe's record of them from another.
		exts = append(exts,
			manifestlocal.DeclareReadRoot(layer.anchor, "skills"),
			manifestlocal.DeclareReadRoot(layer.anchor, "tasks"))
		if info, err := os.Stat(skillsDir); err == nil && info.IsDir() {
			exts = append(exts, skillscontrib.ContributeSkillResources(layer.provider, loader.NewFSLoader(skillsDir), "."))
		}
		if info, err := os.Stat(tasksDir); err == nil && info.IsDir() {
			exts = append(exts, taskcontrib.ContributeTaskResources(layer.provider, loader.NewFSLoader(tasksDir), "."))
		}
	}

	// The pack kinds ktw-agents carries (skill, task) are published once per
	// binary, never per skill/task root or layer above.
	// Leaves() with no kind would answer every leaf: the names are given, and
	// an empty answer adds nothing.
	if leaves := builtin.Leaves("skill", "task"); len(leaves) > 0 {
		exts = append(exts, builtin.ContributeLeafKinds("", leaves...))
	}

	// The private composition (private.go, left out of the open-source build).
	exts = append(exts, privateExtensions...)

	return application.New(application.Config(), exts...)
}

// twoLayerLoaderDirs returns the global + universe layer paths for skills/
// tasks/sync loader contribution. Order matters: global first (lowest
// priority), universe second (higher priority — last-wins on collision per
// the existing skill manager semantics).
//
// Returns one entry if the two paths coincide (e.g. binary running outside
// a universe with no separate global root).
func twoLayerLoaderDirs(env application.AppEnvironment) []struct {
	path     string
	provider string
	anchor   string // the folder the hub resolves path against (mbapi.ReadRoot*)
} {
	type entry = struct {
		path     string
		provider string
		anchor   string
	}
	universe := env.UniverseDir()
	global := env.GlobalHomeDir()
	if global == "" || global == universe {
		return []entry{{path: universe, provider: "user", anchor: mbapi.ReadRootUniverse}}
	}
	return []entry{
		{path: global, provider: "global", anchor: mbapi.ReadRootGlobal},
		{path: universe, provider: "universe", anchor: mbapi.ReadRootUniverse},
	}
}

// sidecarPluginsExt loads the endpoint registry and contributes sidecar plugin factories
// to the agent.plugin domain during AppInit. The sender factory is a stub that errors
// at invocation time — actual transport wiring happens at session/container creation.
var sidecarPluginsExt application.Extension = func(init *application.ApplicationInit) {
	init.AddAppInit("sidecar-plugins", "", func(_ *application.Application) error {
		reg, err := appcore.LoadUniverseSidecars()
		if err != nil {
			slog.Warn("sidecar plugins", "error", err)
			return nil // non-fatal
		}
		if len(reg.Sidecars) == 0 {
			return nil
		}

		stubSenderFactory := agentlocal.StubSenderFactory

		for i := range reg.Sidecars {
			spec := &reg.Sidecars[i]
			manifest := spec.Manifest
			if manifest == nil {
				// No cached manifest. Fetching dynamically here would
				// race with sidecarDiscoveryExt's own fetch; skip the
				// plugin contribution for this sidecar at AppInit time.
				// (Plugins re-resolve at run time when an agent calls
				// the plugin by name; missing one = clean error.)
				continue
			}
			for _, slot := range manifest.KTSP.Slots {
				if slot.SlotType == "" {
					continue
				}
				// Skip if already registered (built-in plugin takes precedence).
				if agentlocal.PluginDomain.Types.Has(slot.Name) {
					continue
				}
				factory := agentlocal.NewSidecarPluginFactory(slot.Name, *manifest, slot, stubSenderFactory)
				td := strategies.TypeDef[agentapi.PluginFactory]{
					DefMeta:  strategies.DefMeta{Name: slot.Name, Label: slot.Name},
					Strategy: "sidecar",
					Impl:     factory,
				}
				if err := agentlocal.PluginDomain.Types.Contribute(td); err != nil {
					slog.Warn("sidecar plugin", "slot", slot.Name, "error", err)
				}
			}
		}
		return nil
	})
}

// THE COMMAND LINE AND THE HUB — one rule: no hub, spawn
// one; a hub, be its client. Every verb connects with ModeFindOrSpawn: a
// running hub is found, else one is spawned, and the other sidecars reach
// it on their first call (the hub spawns them). A hub this command spawned
// serves every command until none uses it, then ends by itself (`--hub stop`,
// the default), or is left running as the universe's hub (`--hub
// keep`, or KTW_HUB=keep; `katyoti universe stop` ends it).
//
// Kept out, ModeReuse: `--help` / `--version` (cobra short-circuits before
// RunE, an owned hub would leak), `llm …` and `version` / `completion` /
// `describe` / `help` (no universe needed), and — crucially — `sc …`:
// `sc ktsp nats` is ktw-agents running AS a hub-managed sidecar, which must
// never boot a second universe.
var noUniverseCommands = map[string]bool{
	"sc": true, "llm": true, "version": true, "completion": true, "describe": true, "help": true,
}

// hubKeepFlag is the persistent flag choosing what a spawned hub does at
// exit; hubKeepEnv the environment form, for a session that sets it once.
const (
	hubKeepFlag = "--hub"
	hubKeepEnv  = "KTW_HUB"
	hubKeep     = "keep"
	hubStop     = "stop"
)

// flagHub is the flag's cobra side: pre-scanned by hubModeFor at AppInit
// (before cobra parses), declared for --help and validation.
var flagHub string

func init() {
	rootCmd.PersistentFlags().StringVar(&flagHub, "hub", hubStop,
		"A hub spawned by this command: stop (ends once no command uses it) or keep (left running as the universe's hub; katyoti universe stop ends it). Env: KTW_HUB.")
}

// localCommands are the verbs that work on the workspace IN THIS PROCESS even
// when a hub runs: the universe-execution commands (`run`, `engine`,
// `terminal`: the process is then the one serving) and the hand-written
// method commands, which call the CWM service directly rather than through a
// generated verb. They behave as they always did; every generated verb is a
// client of the running universe (clientMode).
var localCommands = map[string]bool{
	"run": true, "engine": true, "terminal": true, "sm": true,
	"method-overview": true, "method-act": true, "method-demo": true,
}

// verbOf is the first positional token of the command line — the cobra verb.
func verbOf(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if globalValueFlags[a] && !strings.Contains(a, "=") {
				i++
			}
			continue
		}
		return a
	}
	return ""
}

// A CLIENT OF THE RUNNING UNIVERSE — the other half of the rule. Once a hub is
// reachable (found, or spawned by this command), this process is a CLIENT of
// the universe: it opens no workspace (cwmlocal.Options.ServedElsewhere), and
// every generated verb — cwm, requests, todos, work, action run, … — is sent
// over the hub to the serving ktw-agents, which the hub spawns on the first
// call if none serves yet. One process holds the workspace; the GUI open and
// the command line work together; `action run` reaches any sidecar's method.
// Without a hub (an excluded verb, or no hub binary to spawn) the workspace is
// opened here as before, on this binary's own types only.
//
// The decision is made once per process, the first time it is asked — from
// the CWM factory at AppInit, or when the generated verbs are wired.
var clientModeOnce struct {
	sync.Once
	on bool
}

// clientMode says whether this process is a client of its running universe
// (see above), connecting to the hub to find out.
func clientMode(ctx context.Context) bool {
	clientModeOnce.Do(func() {
		if appcore.ManifestProbeProcess() {
			return
		}
		if !wantsClient(os.Args[1:]) {
			return
		}
		// A verb told another workspace than its universe's works on that one,
		// here: the serving process holds its universe's workspace only, and a
		// verb sent to it would run there, the flag silently ignored.
		served := application.Env().HomeDir()
		if named := namedWorkspace(os.Args, os.Getenv(cwmapi.EnvVarWorkspaceDir)); !servesWorkspace(served, named) {
			slog.Info("workspace: the command names another workspace than its running universe's; working on it in this process",
				"workspace", named, "universe workspace", served)
			return
		}
		// A verb that spawns none (--help, llm, version …) still REUSES a
		// running hub: with the universe up, opening the workspace here would
		// meet the serving process's lock. No hub then: it works here, quietly.
		mode, _ := hubModeFor(os.Args[1:], os.Getenv(hubKeepEnv))
		hc, err := application.UseService[*hubclient.HubClient](ctx)
		if err != nil || hc == nil {
			if mode == hubclient.ModeFindOrSpawn {
				slog.Warn("hub: none reachable; working on the workspace in this process, with this binary's own types only", "error", err)
			}
			return
		}
		clientModeOnce.on = true
	})
	return clientModeOnce.on
}

// namedWorkspace is the workspace folder a command line names — its
// --workspace-dir, else KTW_WORKSPACE_DIR — absolute; "" when it names none.
func namedWorkspace(args []string, env string) string {
	named := scanWorkspaceDirFromArgs(args)
	if named == "" {
		named = env
	}
	if named == "" {
		return ""
	}
	if abs, err := filepath.Abs(named); err == nil {
		return abs
	}
	return named
}

// servesWorkspace says whether the process serving the universe whose
// workspace is served would answer for named: always when named is empty,
// else when both name the same folder.
func servesWorkspace(served, named string) bool {
	if named == "" {
		return true
	}
	a, b := filepath.Clean(served), filepath.Clean(named)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// wantsClient says whether a command line may be a client of a running
// universe: every verb but the serving process itself (`sc …`: `sc ktsp nats`
// IS the process serving the workspace) and the commands that work on the
// workspace here (localCommands).
func wantsClient(args []string) bool {
	verb := verbOf(args)
	return verb != "sc" && !localCommands[verb]
}

// clientCallTimeout is how long a client verb waits for the serving process
// when its method advertises no timeout of its own.
const clientCallTimeout = 10 * time.Minute

// actionRunCeiling is how long `action run` waits for the serving process: the
// serving ActionService bounds the target by the target's own advertised
// timeout (prove-wait's 16 minutes, a build's), which action/run cannot know
// here - so the client waits up to this ceiling, never less than it needs.
const actionRunCeiling = time.Hour

// clientTimeout is how long a client verb waits for slot/method, which
// advertises timeout (0: none).
func clientTimeout(slot, method string, timeout time.Duration) time.Duration {
	if slot == "action" && method == "run" && timeout < actionRunCeiling {
		return actionRunCeiling
	}
	if timeout <= 0 {
		return clientCallTimeout
	}
	return timeout
}

// installClientDispatch, once the generated verbs exist, routes them over the
// hub when this process is a client: provider:/slot/method with the JSON the
// flags built, the method's timeout, through the retrying sender (a cold
// serving ktw-agents is spawned by the hub on the first call).
func installClientDispatch(app *application.Application) {
	ctx := app.Context()
	if !clientMode(ctx) {
		return
	}
	hc, err := application.AppUseService[*hubclient.HubClient](app)
	if err != nil {
		return
	}
	tenantID, universeID := cenv.ResolveTenantUniverse(application.Env().UniverseDir())
	sender := messagebus.NewRetrySender(messagebus.NewSender(hc.MessageBus(), tenantID, universeID), "")
	cli.SetRemoteDispatcher(func(ctx context.Context, provider, slot, method string, timeout time.Duration, data []byte) ([]byte, error) {
		path := provider + ":/" + slot + "/" + method
		// A method with no advertised timeout ran here without one: over the
		// hub it waits as long (clientCallTimeout), never the bus's few
		// seconds — a check running tests, a run, a long query. Ctrl+C ends
		// the wait.
		timeout = clientTimeout(slot, method, timeout)
		opts := []ktsp.CallOption{ktsp.WithTimeout(timeout)}
		out, err := sender.Call(ctx, path, json.RawMessage(data), opts...)
		if err != nil {
			return nil, fmt.Errorf("%s (sent to the process serving this universe; the command line is its client while a hub runs): %w", path, err)
		}
		return out, nil
	})
	slog.Debug("hub: this command is a client of the running universe; its verbs go to the serving process", "hub", hc.NATSURL())
}

// globalValueFlags are the persistent flags that consume the following token
// as their value, so hubModeFor skips it when scanning for the verb
// (handles `ktw-agents --home X run …`). `--flag=value` forms consume nothing
// extra and are skipped by the generic flag branch.
var globalValueFlags = map[string]bool{"--home": true, "--universe": true, "--workspace-dir": true, hubKeepFlag: true}

// hubModeFor reads the command line (os.Args[1:]) and the KTW_HUB variable
// and says how this command connects to the hub, and whether a hub it spawns
// is kept (detached) at exit. It runs at AppInit, before cobra parsed
// anything — mirroring the AutoKTSP flag-prescan pattern.
func hubModeFor(args []string, env string) (mode hubclient.Mode, keep bool) {
	keep = env == hubKeep
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "--version" {
			return hubclient.ModeReuse, false
		}
	}
	verb := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			if name, value, cut := strings.Cut(a, "="); cut {
				if name == hubKeepFlag {
					keep = value == hubKeep
				}
				continue
			}
			if globalValueFlags[a] && i+1 < len(args) {
				if a == hubKeepFlag {
					keep = args[i+1] == hubKeep
				}
				i++ // skip this flag's value token
			}
			continue
		}
		if verb == "" {
			verb = a
		}
	}
	if verb == "" || noUniverseCommands[verb] {
		return hubclient.ModeReuse, false
	}
	return hubclient.ModeFindOrSpawn, keep
}

// newSharedHub is the factory for the process-lifetime *HubClient singleton.
// A spawned hub is owned unless kept: it outlives this command while other
// commands use it and ends once none does; a hub found running is reused. The connect uses context.Background() so the hub connection isn't
// bound to any request ctx — it lives until the container closes.
func newSharedHub(_ context.Context) (*hubclient.HubClient, error) {
	mode, keep := hubModeFor(os.Args[1:], os.Getenv(hubKeepEnv))
	hc, err := hubclient.ConnectToHub(context.Background(), hubclient.ConnectOptions{
		DataDir: application.Env().DataDir(),
		Mode:    mode,
		Detach:  keep,
	})
	if err == nil && hc.Spawned() {
		if keep {
			slog.Info("hub: spawned and kept running (--hub keep); `katyoti universe stop` ends it", "pid", hc.LockFile().PID)
		} else {
			slog.Debug("hub: spawned for this command; it ends once no command uses it (--hub keep leaves it running)")
		}
	}
	return hc, err
}

// sidecarDiscoveryExt is what remains of this binary's universe wiring once
// the assembly itself became generic: obtain the universe view the same way
// any consumer does, then do the one thing that is specific to ktw-agents —
// aggregate the universe's tasks into its TaskService.
//
// Everything the old block did by hand — paging the aggregator's body-free
// index, regrouping it per provider, building the in-process self manifest,
// wiring the content lane, subscribing to change — now lives in
// manifestuniverse.UniverseCatalogExtension, where any consuming binary can reach it.
var sidecarDiscoveryExt application.Extension = func(init *application.ApplicationInit) {
	manifestuniverse.UniverseCatalogExtension(manifestuniverse.WithChangeSubscription())(init)

	// Bootstrap: consume the universe view published at AppInit. Phases run
	// strictly in sequence, so the catalog is always present here.
	init.AddBootstrap("sidecar-discovery", ".tasks", func(app *application.Application) error {
		catalog, cerr := application.AppUseService[mbapi.Catalog](app)
		if cerr != nil {
			slog.Warn("sidecar discovery: universe catalog unavailable; skipping task aggregation", "error", cerr)
			return nil
		}
		// One CatalogTaskLibrary over the whole universe view: `tasks list`
		// is one walk of the index and issues no RPC per sidecar.
		if err := tasklocal.WrapTaskLibraryWithRemotes(app, tasklocal.NewCatalogTaskLibrary(catalog)); err != nil {
			slog.Warn("sidecar discovery: task aggregation failed", "error", err)
		}
		return nil
	})
}

// cwmWorkspaceRootProvider resolves the CWM workspace folder for
// the cwmlocal Extension. Called at AutoKTSPContainer-AppInit
// time, which fires BEFORE Cobra parses flags — so we scan
// os.Args directly for `--workspace-dir` (mirroring cenv.Preflight
// for `--home`/`--universe`). The `flagWorkspaceDir` package
// variable is also consulted in case service-construction is
// somehow deferred past flag parsing in a future code path.
func cwmWorkspaceRootProvider() (string, error) {
	explicit := flagWorkspaceDir
	if explicit == "" {
		explicit = scanWorkspaceDirFromArgs(os.Args)
	}
	root, err := cwmapi.DiscoverWorkspaceRoot(cwmapi.DiscoverOptions{
		Explicit: explicit,
	})
	if err != nil {
		return "", err
	}
	return root.Path, nil
}

// scanWorkspaceDirFromArgs returns the value of `--workspace-dir`
// from a raw argv slice. Supports both `--workspace-dir <value>`
// and `--workspace-dir=<value>` forms. Returns "" when the flag is
// absent or its value is missing.
func scanWorkspaceDirFromArgs(args []string) string {
	const flag = "--workspace-dir"
	for i, arg := range args {
		switch {
		case arg == flag && i+1 < len(args):
			return args[i+1]
		case len(arg) > len(flag)+1 && arg[:len(flag)+1] == flag+"=":
			return arg[len(flag)+1:]
		}
	}
	return ""
}

// humanGateDefaultExt registers a default HumanGate (CLI on stderr).
// TUI mode overrides this with TUIPrompter.AsHumanGate() before starting the task.
var humanGateDefaultExt application.Extension = func(init *application.ApplicationInit) {
	init.AddAppInit("humangate.default", "", func(app *application.Application) error {
		prompter := humangate.NewCLIPrompter(os.Stderr, os.Stderr)
		application.PutService[hgapi.HumanGate](app, "default", hgapi.HumanGate(prompter))
		return nil
	})
}

// ktspInfraExt registers the KTSP engine infrastructure services:
// EventBus (for Subscribe/Notify). Machine routing happens through
// MachineDomain.Extension() above.
var ktspInfraExt application.Extension = func(init *application.ApplicationInit) {
	init.AddAppInit("ktsp.infra", "", func(app *application.Application) error {
		bus := ktspengine.NewMemoryEventBus()
		application.PutService[ktspengine.EventBus](app, "default", bus)
		return nil
	})
}
