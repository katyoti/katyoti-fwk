# appcore/sidecar — Sidecar Contract Package

This package implements the sidecar contract described in [modules/sidecars/tools.md §1](../../../modules/sidecars/tools.md#1-the-sidecar-builder--sidecarkitnewsidecar). It provides three services that every ktw-* sidecar publishes, plus a bootstrap client for configuring sidecars before KTSP is available.

## Package Layout

```
sidecar/
├── api/              Interfaces + types (imported by both sides)
│   ├── info.go       Sidecar service — identity + introspection (InfoReport)
│   ├── config.go     ConfigService — 7 methods for config lifecycle
│   └── llm.go        LLMService — 4 methods for LLM-facing interface
├── client/           Clients for CALLING sidecar services
│   └── cli_client.go Bootstrap client: runs binary via exec.Command + JSON
├── internal/         Server-side implementations (used inside sidecars)
│   ├── default_info.go    Generic Sidecar impl (reads PublishedServicesPoint)
│   ├── default_config.go  Generic ConfigService impl (absorbs check/setup logic)
│   └── default_llm.go     Generic LLMService impl (tool dispatch + skill loading)
└── README.md
```

## Two Consumers, Two Paths

### For sidecars (server side)

Every sidecar wires one line in `buildExtensions`:

```go
appcore.SidecarExt(efs)  // publishes Sidecar + ConfigService + LLMService
```

This uses the generic implementations from `internal/`. No sidecar-specific code needed — the impls read from `PublishedServicesPoint`, `embed.FS`, and `application.Env()` to answer all queries.

The services are exposed via auto-CLI, KTSP, and HTTP alongside the sidecar's domain services. CLI verbs are grouped:

```
<sidecar> config check|do|describe|get-default|get|validate|materialize
<sidecar> skills list|get-instruction|get-reference
<sidecar> llm    info|call-tool|list-tools|describe-tool
<sidecar> <domain-tool>   (flat for single-service sidecars)
```

### For ktw-agents (client side)

ktw-agents uses the `client/` package to discover and configure sidecars **before KTSP is available**:

```go
// 1. Discover the ktw-* sidecars installed beside the running binary
lookup, _ := client.NewNeighborLookup()
sidecars := lookup.Lookup()
sidecar := sidecars[0]

// 2. Bootstrap client — runs the binary, no KTSP needed
cli := client.NewCLIClient(sidecar.Path)

// 3. Introspect (works unconfigured)
info, _ := cli.Info(ctx)
fmt.Println(info.Name, info.Version, len(info.Services), "services")

// 4. Check status (works unconfigured)
report, _ := cli.Check(ctx)
for _, dep := range report.Deps {
    fmt.Println(dep.Name, dep.OK)
}

// 5. Configure (writes config files — transitions sidecar to operational)
cli.Materialize(ctx, api.ConfigMaterializeInput{})

// 6. Done — sidecar is now configurable. Switch to KTSP for operational use.
```

**After configuration**, ktw-agents connects to the sidecar via KTSP (`<sidecar> serve` starts the KTSP server). From that point, all interactions use the standard KTS wire protocol — the CLI client is no longer needed.

## The Two States

```
UNCONFIGURED                    OPERATIONAL
┌─────────────────────┐        ┌─────────────────────┐
│ Binary on disk      │        │ Configured           │
│ No config files     │ ────►  │ KTSP server running  │
│ No KTSP server      │ config │ Full services        │
│                     │   do   │ Remote-callable      │
│ Access: CLI only    │        │ Access: CLI+KTSP+HTTP│
│ Client: CLIClient   │        │ Client: KTSP stubs   │
└─────────────────────┘        └─────────────────────┘
```

## Services Reference

### Sidecar (identity + introspection)

| Method | Tool name | Purpose |
|--------|-----------|---------|
| `Info` | `sidecar-info` | Structured JSON report: name, version, services, tools, config, skills, capabilities |

### SidecarConfigService (config lifecycle — 5 verbs)

The SidecarConfigService has five methods; its CLI verbs live under the
`sc config` group ([the command-line decision](../../../modules/sidecars/decisions/OD-003-how-is-the-command-line-of-every-sidecar-binary.cwm.md)).

| Method | Tool name | CLI | Purpose |
|--------|-----------|-----|---------|
| `Get`      | `config-get`      | `sc config get NAME [--default]` | Read live or embedded default config |
| `Validate` | `config-validate` | `sc config validate NAME`        | Check proposed config (no write) |
| `Set`      | `config-set`      | `sc config set [--reset]`        | Write config to disk (idempotent) — formerly `Materialize` |
| `Check`    | `config-check`    | `sc config check`                | Canonical health-check (deps, env, files, hints) |
| `Setup`    | `config-setup`    | `sc config setup`                | Interactive setup — formerly `Do` |

`Describe` was removed 2026-05-10 (data now lives in
`ResourceService.List(type=config-file|config-program|env)`).
`GetDefault` was folded into `Get` (flag `Default bool`) 2026-05-23.

### LLMService (LLM-facing interface)

The CLI uses `<group> <noun> <verb>`
ordering with subgroup aliases (`tls`, `rsc`). The Go interface +
KTSP method names are unchanged.

| Method | Tool name | CLI | Purpose |
|--------|-----------|-----|---------|
| `Info`              | `llm-info`                | `llm info`                | Usage guide + calling conventions |
| `CallTool`          | `llm-call-tool`           | `llm tools call NAME '<json>'` | Generic tool dispatch |
| `ListTools`         | `llm-list-tools`          | `llm tools list`          | All tools with schemas |
| `DescribeTool`      | `llm-describe-tool`       | `llm tools describe NAME` | One tool's JSON schema |
| `ListResources`     | `llm-list-resources`      | `llm resources list [--type T]` | Contributed resources |
| `GetResource`       | `llm-get-resource`        | `llm resources get URI`   | Resource body by URI |
| `ListResourceTypes` | `llm-list-resource-types` | `llm resources types`     | Distinct resource types |

A new `llm manifest` verb (no service method) aggregates `Sidecar.Info` +
`LLMService.ListTools` + `LLMService.ListResourceTypes` into a single
one-shot discovery document.

## Related

- [How is the command line of every sidecar binary organised?](../../../modules/sidecars/decisions/OD-003-how-is-the-command-line-of-every-sidecar-binary.cwm.md) — the three command families (domain / `sc` / `llm`)
- [How does a sidecar binary declare its wiring?](../../../modules/sidecars/decisions/OD-001-how-does-a-sidecar-binary-declare-its-wiring.cwm.md) — when to use `sidecarkit.NewSidecar` vs explicit list
- [A binary's cmd folder holds root.go and bootstrap.go only](../../../modules/sidecars/rules/R-001-a-binary-s-cmd-folder-holds-root-go-and.cwm.md) — bootstrap wiring pattern
- [authoring a service](../../../modules/framework/authoring-a-service.md#5-publication--local) — how domain services are published
- `SidecarExt(efs)` lives at `internal/appcore/sidecar_ext.go` (the one-liner facade)
