# tools/api

Agent tool API: the `Tool` interface every agent-callable tool implements,
the `NewFunctionTool` / `NewToolGroup` builders, and the plugin bridges
that turn tools into `agentapi.AgentPlugin` values.

See the decision [What shapes does an agent tool take, and through which endpoints is it registered?](../../../../modules/tools/decisions/OD-001-what-shapes-does-an-agent-tool-take-and-through.cwm.md).

---

## Two tool shapes

### Function tool — single typed handler

```go
type MyInput struct {
    Query string `json:"query" jsonschema:"Search query"`
    Limit int    `json:"limit,omitempty" jsonschema:"Max results"`
}

type MyOutput struct {
    Results []string `json:"results"`
}

func handler(ctx context.Context, in MyInput) (MyOutput, error) {
    // ...
    return MyOutput{Results: results}, nil
}

tool := api.NewFunctionTool[MyInput, MyOutput](
    "my_tool", "Search for things.", handler,
    api.WithReadOnly(),
)
```

### Tool group — Go interface, one tool per method

```go
type MyTools interface {
    Search(ctx context.Context, in SearchInput) (SearchOutput, error)
    Index (ctx context.Context, in IndexInput)  (IndexOutput, error)
}

type impl struct{ /* ... */ }
func (i *impl) Search(ctx context.Context, in SearchInput) (SearchOutput, error) { /* ... */ }
func (i *impl) Index (ctx context.Context, in IndexInput)  (IndexOutput,  error) { /* ... */ }

tools := api.NewToolGroup[MyTools](
    "my", "My ops.", &impl{},
    api.WithReadOnly(),
)
// → []api.Tool{ "my-search", "my-index" }
```

Each method becomes one `Tool` named `<group>-<method-kebab>` (tool names are
kebab-case). Override the prefix with `WithGroupPrefix("")`
(no prefix) or `WithGroupPrefix("code-")` (another kebab prefix).

A group impl that holds an external resource (LSP server, REPL, MCP, etc.)
can implement `ConnectableToolGroup` for `Open` / `Close` lifecycle:

```go
func (i *impl) Open (ctx context.Context) error { /* connect */ return nil }
func (i *impl) Close(ctx context.Context) error { /* disconnect */ return nil }
```

The plugin strategy is responsible for calling `Open` before the first tool
invocation and `Close` via the scoped-service teardown.

---

## Plugin bridges

### Function tool → plugin strategy (the simple path)

```go
var pluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
    "my_tool", "Search for things",
    func() (agentapi.PluginFactory, error) {
        return api.NewToolPluginFactory(tool), nil
    },
)

func Extension(init *application.ApplicationInit) {
    init.AddContribute("tools", ".my_tool", func(app *application.Application) error {
        return agentapi.ContributePluginStrategy(app, pluginStrategy)
    })
}
```

`api.ToolExtension(tool)` is a one-liner shortcut that produces the same
`PluginStrategyDef`.

### Tool group → composite plugin (DI-resolved impl)

```go
var pluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
    "my", "My tools",
    func() (agentapi.PluginFactory, error) {
        return &agentapi.FuncFactory{
            FactoryName: "my",
            CreateFunc: func(ctx context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
                impl := application.UseService[MyTools](ctx)
                if c, ok := impl.(api.ConnectableToolGroup); ok {
                    if err := c.Open(ctx); err != nil { return nil, err }
                }
                methods := api.NewToolGroup[MyTools]("my", "My ops.", impl, api.WithReadOnly())
                return []agentapi.AgentPlugin{api.NewToolGroupPlugin("my", "", methods)}, nil
            },
        }, nil
    },
)
```

The group impl is registered as a service via `publish.PublishService` so
it benefits from the standard config-loading pipeline and can declare
infrastructure dependencies (config files, API keys, binary deps).

---

## Options

| Option | Effect |
|---|---|
| `WithReadOnly()` | tool is read-only and concurrency-safe; default permission auto-allows |
| `WithMutating()` | tool mutates state (fail-closed default) |
| `WithDestructive()` | mutating + escalated permission prompt |
| `WithAliases(names...)` | alternative names for HandleToolCall lookup |
| `WithEnabledFunc(fn)` | runtime feature gate |
| `WithDescriber(fn)` | custom per-call description for permission UI |
| `WithMaxResultSize(n)` | byte limit for result data (default 32 KB) |
| `WithUnlimitedResult()` | no result-size limit |
| `WithCancelOnInterrupt()` | tool aborts on user-submitted new message |
| `WithDeferred(hint)` | hide tool from initial system prompt; load via ToolSearch |
| `WithPrompt(text)` | system-prompt contribution |
| `WithPermissionCheck(fn)` | custom permission policy |
| `WithAllowPermission()` | always allow |
| `WithAskPermission()` | always ask |
| `WithGroupPrefix(prefix)` | tool group only — override default `<group_name>_` prefix |

Defaults are **fail-closed** (spec FR-01): a tool created without options
is mutating, not concurrency-safe, asks for permission, blocks on interrupt,
32 KB result limit.

---

## Key types

| Symbol | Purpose |
|---|---|
| `Tool` | the 14-method interface every agent tool implements |
| `FunctionTool[I,O]` | single-handler tool produced by `NewFunctionTool` |
| `ConnectableToolGroup` | optional `Open`/`Close` interface for groups holding external resources |
| `PermissionResult`, `PermissionBehavior`, `PermissionFunc` | permission gate types (FR-03) |
| `InterruptBehavior` | block / cancel on interrupt (FR-07) |
| `ToolOptions`, `ToolOption`, `With*` | declarative tool configuration |
| `NewFunctionTool[I,O]` | function tool builder |
| `NewToolGroup[T]` | reflection-based group builder |
| `NewToolPluginFactory(t)` | wrap a function tool as an `agentapi.PluginFactory` |
| `NewToolPlugin(t)` | wrap a function tool as an `agentapi.AgentPlugin` |
| `NewToolGroupPlugin(name, sysmsg, tools)` | composite plugin routing by tool name + alias |
| `ToolExtension(t)` | one-liner: function tool → `agentapi.PluginStrategyDef` |
| `AllowFunc / AskFunc / DenyFunc / PolicyFromOptions` | permission policy helpers |
