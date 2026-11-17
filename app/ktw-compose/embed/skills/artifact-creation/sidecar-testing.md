---
name: sidecar-testing
advertised: false
description: "Read-only verification battery for a ktw-* sidecar binary; probes the command surface and writes a Markdown quality report. Load to validate a sidecar after a build or before release."
iconName: stethoscope
icon: "🩺"
hashtags: [testing, sidecar, contributions, llm-surface, qa]
category: development
---

# Sidecar Testing — read-only verification battery

This skill runs a comprehensive READ-ONLY probe of a ktw-* sidecar
binary, validates every facet of its command surface, and writes a
structured Markdown report to `test_results/<binary-name>-<YYYYMMDD-HHMMSS>.md`.

## Strict constraints

1. **Read-only binary commands**. Use ONLY discovery / introspection /
   read verbs on the binary under test: `list`, `get`, `describe`,
   `info`, `manifest`, `types`, `check`, `dirs`. **NEVER** invoke
   commands that register, create, delete, run a remote query, modify
   state, or spawn a long-lived server. If a verb's purpose is unclear,
   skip it and note it under "needs human review" in the report.
2. **No Write/Edit/MultiEdit to repo files** other than the test
   report itself. The report goes under `test_results/`. Don't pollute
   the repo with captured stdout files.
3. **One bounded long-running process allowed** — the Section 3.5
   `$BIN sc ktsp stdio` server. Start it once, run every stdio probe
   against the same session, then close stdin to terminate it. Total
   stdio session must complete in ≤ 60 s. Every OTHER command runs
   synchronously with a 30-second timeout. If a command hangs, mark
   it KO and move on. No other long-running surfaces (HTTP server,
   MCP bridge, NATS) are started by this skill.

## Inputs

| Input | Default | Notes |
|---|---|---|
| `BIN` | — (e.g. `./bin/ktw-example.exe`) | Full path to the sidecar binary |
| `BIN_NAME` | derived from `BIN` (strip path + `.exe`) | Used in the report filename |

If the user invokes the skill without specifying a binary, ask which
sidecar binary to test before running anything.

## URI conventions (CRITICAL — do not confuse the two forms)

The framework uses **two distinct URI shapes** to discriminate
**callable tools** from **readable resources**:

| Surface | URI shape | Slash after colon? | Examples |
|---|---|---|---|
| **Tool** (callable verb) | `<provider>:<slot>/<method>` | **NO** | `ktw-example:resources/get`, `ktw-example:example/search` |
| **Resource** (readable Contribution) | `<provider>:/<type>/<id>` | **YES** | `ktw-example:/skill/example`, `system:/data-type/contribution` |

The presence or absence of the slash after `:` is the **only**
syntactic differentiator. Test consumers (LLMs, CLIs, the bus
parser) rely on this distinction. When the skill reports URIs in
the test_results document, it MUST preserve the exact form returned
by the binary — never normalize one form into the other.

This convention also tells you which fetch verb to use:

- Tool URIs → describe the tool's schema for `<tool-uri>`, or
  call the `<tool-uri>` tool with `<json-args>` (the latter NOT exercised
  by this read-only battery for state-mutating tools).
- Resource URIs → fetch the resource entry for `<resource-uri>` via the
  `llm` group (metadata / manifest entry) and read the resource content
  for `<resource-uri>` (raw content — the lane skills and references are
  loaded through). The operator-surface (`sc`) resource twins must emit
  identical JSON (parity probe).

A test command that swaps these (e.g. tries `resources get` on a
tool URI) is itself a defect of the test, not the binary — fix the
probe.

## Architecture context (2026-06-05 — what this skill does NOT test)

This skill exercises ONE sidecar binary in isolation through its
direct CLI / LLM / stdio surfaces. It does NOT test the hub or
cross-binary catalog. Knowing what's outside scope avoids spurious
KO entries when the failure is actually upstream:

- **Hub manifest cache lives at `<CacheDir>/sidecars/<sidecar>/`**
  (universe-agnostic, post 2026-06-05 redesign — NOT
  `<DataDir>/cache/` or the prior flat `<DataDir>/inventory/`). The
  skill never reads these files — it queries the live binary directly.
  If you suspect cache staleness rather than a binary defect, run
  `bin/ktw-hub catalog refresh <sidecar>` BEFORE this skill.
- **Hub owns the cache; consumers go through the bus** —
  `ktw-hub:/CatalogService/*`. ktw-gui's KTSPBrowser and ktw-agents'
  bootstrap both use this path; only ktw-hub itself touches cache
  files. If a binary you're testing reads catalog files directly,
  flag it as a layering violation (separate from any per-binary KO).
- **System data-type bodies are `DataTypeBody{format, source}`
  envelopes** (post 2026-06-05 Bug 3 fix). Raw YAML re-marshalled as
  JSON used to be a silent ingest failure at the hub Phase 2. This
  skill's Section 4 now has a dedicated probe — if any system
  data-type body returns `{"name": "...", ...}` directly instead of
  `{"format": "yaml", "source": "..."}`, flag it loudly: it's the
  symptom that drops it from `SchemaService.RegisterDataType` and
  triggers `catalog.Reindex: data-type not found` warnings hub-side.

## Surfaces under test (CRITICAL — name them by their proper role)

Every published method may be reachable on up to FIVE surfaces. The
skill exercises the first three explicitly and records the last two as
"available NOT TESTED" pending follow-up work.

| Surface | What it is | Tested by this skill? |
|---|---|---|
| **CLI** | `$BIN sc <group> <verb>` / `$BIN <domain> <verb>` — operator-facing rich-or-JSON CLI | YES — Section 2 |
| **LLM** | call the `<provider>:<slot>/<method>` tool with `<json>` via the `$BIN llm` group — JSON-in/JSON-out adapter that LLM agents drive | YES — Section 3 |
| **stdio method-call** | `$BIN sc ktsp stdio` — long-running JSON-RPC 2.0 server on stdin/stdout; the canonical inter-process bus surface | YES — Section 3.5 |
| **HTTP** | `$BIN sc http serve` — REST surface mirroring the method-call container | NO — record availability only |
| **MCP** | Model Context Protocol bridge (separate binary) — mirrors method-call slots as MCP tools | NO — record availability only |

## Scope discriminator (CRITICAL — local vs system vs sidecar)

Every Contribution carries a `Provider` field that tells you whether
the entry is bus-visible or binary-private:

| `Provider` value | Meaning | Visible on bus? | Counts toward catalog? |
|---|---|---|---|
| `system` | bootstrap metadata embedded by every binary (contribution-type / data-type / well-known kinds) | yes | once per universe (deduped) |
| `<binary-name>` (e.g. `ktw-example`) | sidecar-owned contribution — the standard scope | yes | once per binary |
| `local` | binary-private — only visible to the local CLI surface; filtered out on the wire | NO | never |

The skill MUST classify every Contribution by Provider in the report.
Bus-side callers (hub catalog refresh, other sidecars, remote agents)
see only `system` + `<binary-name>` entries unless they explicitly
pass `includeLocal: true`. Local-CLI commands set `includeLocal: true`
so operators see everything, but the matrix in Section 8 must split
local vs published columns.

## Test battery — 8 sections

Run each section in order. For every command:

- Capture **exit code** (0 = OK, non-zero = KO)
- Verify **output format** matches expectations (rich vs JSON)
- Verify **information completeness** (no empty payloads, no
  obvious truncation, all expected sections present)
- Record any **surprises** (oddly-formatted output, unexpected
  flags, missing data) in that section's findings

### Section 1 — Top-level CLI discovery

Default format: **rich text**.

| Command | Expected behaviour |
|---|---|
| `$BIN --help` | Lists every top-level command + every flag, rich text |
| `$BIN version` or `$BIN --version` | Single semver-like line, rich text |
| `$BIN help <cmd>` for at least one cmd | Per-command help, rich text |

Output-format probes (run each command twice and compare):

```bash
$BIN --help                    # → rich text
$BIN --help --out json 2>&1    # → cobra design: NOT a JSON surface; expected to fail
```

**Expected behaviours (NOT KO):**
- **`$BIN --help --out json` at root is NOT supported and is EXPECTED to fail.**
  `--help` is cobra's built-in usage printer — it never routes through the
  application's `console` package. `--out` is honored only by sub-commands
  that explicitly add it via `console.AddOutputFlag(cmd)`. Mark this probe
  as `ℹ️ EXPECTED` (informational), not ❌ KO. The same applies to
  `$BIN <cmd> --help --out json` for any sub-command — help is rich-only
  by cobra design.
- `$BIN --version` may or may not be wired alongside `$BIN version`. Some
  binaries set cobra's `Version` field (which adds `--version`), others
  only ship the sub-command. Treat absence as a low-priority gap, not KO.

Note any commands that don't accept `--out`.

### Section 2 — Operator surface (sidecar / sc group)

Default format: **rich text**. `--out json` switches to JSON.

| Command | Verify |
|---|---|
| `$BIN sc dirs` | Resolved directories table; presence of HomeDir + ConfigDir + DataDir |
| `$BIN sc dirs --out json` | Same data, JSON shape |
| `$BIN sc config check` | Dependency report; flag missing API keys / configs without crashing |
| the operator-surface skills-list probe (`$BIN sc <skills-group>`) | Every skill listed with name + description |
| `$BIN sc resources types` | Distinct kebab discriminators |
| the operator-surface resources-list probe (`resources/list` method) | Metadata-only list of every contribution |
| the same resources-list probe with `--with-body` | Same list with Body included |
| the same resources-list probe with `--out json` | JSON variant of list |
| `$BIN sc resources manifest` | Full descriptor (slot types + slots + contributions) |
| `$BIN sc ktsp describe` | Slot catalogue with methods + tags |
| `$BIN sc ktsp describe --out json` | JSON variant |

For each, verify `--out json | jq -e .` exits 0 (syntactically valid JSON).

### Section 3 — LLM-driving surface (llm group, JSON-default)

Default format: **JSON**. `--out rich` switches to rich text.

| Command | Verify |
|---|---|
| `$BIN llm info` | JSON object containing sidecar identity, calling conventions, tool list, skill summary, usage guide |
| `$BIN llm manifest` | JSON identity + tools + resource types |
| the `llm` tools-list probe (`tools/list` method) | JSON array of tool URIs + descriptions |
| the same tools-list probe with `--with-schema` | Same list, each entry has a JSON schema |
| describe one tool's schema for `<URI>` (`tools/describe` method) | One tool's full schema (test ≥ 3 distinct tools) |
| the `llm` resources-list probe (`resources/list` method) | JSON array of all resources |
| the same resources-list probe with `--type skill` | Filtered to skills only |
| the same resources-list probe with `--type data-type` | Filtered to data-types only |
| the same resources-list probe with `--type service` | Filtered to services only |
| `$BIN llm resources types` | Distinct types |
| fetch one resource entry for `<URI>` (`resources/get` method) | One resource entry (test ≥ 3 resources of different types) |
| read the resource content for `<URI>` (the resource-read probe) | The resource's CONTENT — for a reference the raw file bytes, for a skill the body with instructions. KO if empty for any existing URI |
| the same resource-read probe for `<URI>` with `--into <tmpfile>` | Writes the content to the file, prints byte count |

Format-switch probes:

```bash
$BIN llm info               # → JSON
$BIN llm info --out rich    # → rich text (if supported)
```

If `--out rich` isn't honored on the llm subgroup, note it as a gap.

### Section 3.5 — method-call stdio bus surface (JSON-RPC 2.0)

The stdio JSON-RPC server is the **canonical inter-process bus
surface** — every consumer that's NOT in-process with the sidecar
(ktw-hub catalog refresh, remote agents, the orchestrator's
`ktransport.StartStdio`, the smoke tests in
`internal/extensions/terminal/local/`) talks to the sidecar this way.
A binary whose CLI/LLM surfaces work but whose stdio surface returns
empty results IS BROKEN — that was the Phase E §5b regression.

Drive the server like the smoke tests do:

1. Spawn `$BIN sc ktsp stdio` as a background subprocess, attach
   stdin / stdout / stderr pipes.
2. For each test request, write a single-line JSON-RPC frame to stdin:
   `{"jsonrpc":"2.0","id":"<test-id>","method":"<slot>/<method>","params":{...}}\n`
3. Read frames from stdout, scan until the response matching `<test-id>`
   appears (timeout 20 s — KO on timeout).
4. Assert `error` is absent and `result` decodes to the expected shape.
5. After all probes, close stdin (the server exits) and verify the
   process returns code 0.

Required probes (every binary must answer all of these):

| `<slot>/<method>` | Params | Expected | Notes |
|---|---|---|---|
| `resources/types` | `{}` | non-empty string array | identity check — must match `$BIN sc resources types` |
| `resources/list` | `{}` | `result.resources` array; **MUST NOT contain `Provider == "local"` entries** | bus default behavior — local scope filtered out |
| `resources/list` | `{"includeLocal": true}` | same set as the local-CLI variant | parity probe |
| `resources/list` | `{"filter": {"types": ["skill"]}}` | every entry has `type == "skill"` | filter validation |
| `resources/get` | `{"uri": "<one-of-the-listed-URIs>"}` | full Contribution with non-empty Body | round-trip |
| `resources/manifest` (alias `get-manifest`) | `{}` | object with `contributions` + `slotTypes` + `slots` + `hash` | full descriptor |
| `sidecar/info` | `{}` | identity + services + skills + capabilities **OR** RPC error `Slot not found: sidecar` (see WithLocal branch below) | identity slot — sidecar contract |

**`sidecar/info` probe branching (Phase E §6 — WithLocal classification rule, 2026-06-06)**:

Before calling `sidecar/info` over stdio, INSPECT the service-scope
view to decide which outcome is correct:

```bash
# list service-type resources as JSON (operator-surface resources-list, --type service --out json),
# then filter with jq for the sidecar service's provider:
... | jq -r '[.contributions[] | select(.id == "sidecar") | .provider] | first // "absent"'
```

- Output `"<provider>"` (e.g. `"ktw-example"`) → the Sidecar service is
  bus-exposed (`WithKTSPContainer`). Run the `sidecar/info` stdio
  probe; expect the full InfoReport. Missing or empty → ❌ KO.
- Output `"local"` → the Sidecar service is bus-private
  (`WithLocal()`). Run the `sidecar/info` stdio probe; expect the
  RPC error `Slot not found: sidecar`. Mark as ℹ️ EXPECTED, NOT KO.
  In this case, identity is still available via the static
  `<provider>:/sidecar-info/default` Contribution (Phase E-4 dual
  model) — verify THAT resolves instead.
- Output `"absent"` → the binary doesn't declare the Sidecar service
  at all (unusual; `appcore.SidecarExt` was not wired). Flag as
  ❌ KO with reason "Sidecar service not declared — bootstrap gap".

Same decision rule applies to ANY service the skill probes by slot
name (`config/*`, `llm/*`, etc.): if `local:/service/<id>` is present
and `<provider>:/service/<id>` is absent, the slot is intentionally
off-bus and slot probes return RPC error ℹ️ EXPECTED.

Comparison probes (REGRESSION GATES):

- Run the operator-surface resources-list probe filtered to `--type task --out json`
  (CLI) and the equivalent `resources/list {"filter":{"types":["task"]}}`
  request (stdio). The `id` sets MUST be identical. If they differ,
  flag the gap loudly — that was the Phase E §5b bug (stdio returned 0
  entries because the request ctx wasn't derived from `cmd.Context()`).
- Call the `<provider>:resources/list` tool with `{}` via the `llm` group and
  run the equivalent stdio request. The `result.resources` array MUST
  match.

Local-scope verification:

- Pick a known-local service (e.g. `<provider>:/service/skill-manager`
  if the binary publishes SkillManager via `WithLocal()`).
- Verify it IS in `resources/list {includeLocal:true}` over stdio.
- Verify it is NOT in `resources/list {}` over stdio (default
  bus-safe).
- Verify it IS in the operator-surface resources-list probe filtered to
  `--type service --out json` (the local CLI defaults to includeLocal=true).

KO conditions:
- Any required stdio probe returns `{"contributions":[]}` while the
  CLI returns entries → **KO** (stdio↔CLI parity gap).
- `Provider == "local"` entry appears in default `resources/list`
  over stdio → **KO** (bus boundary leak).
- The process hangs or returns a JSON-RPC error → **KO**.

### Section 4 — Contribution coverage

Goal: every contribution is reachable through at least one path.

1. Fetch types: `$BIN sc resources types`
2. For each type:
   - List entries from the resources-list JSON, filtered by jq: `... | jq '[.contributions[] | select(.type == "<TYPE>")]'`
   - Verify count > 0 (or document the type as absent)
   - Pick the first 2 entries; for each, run `$BIN sc resources get <URI>` and verify the Body decodes
3. **System contributions** (Phase D-4 ADR + Phase E-4 additions):
   - Filter for `Provider == "system"` in the resources-list JSON, via jq:
     `... | jq '[.contributions[] | select(.provider == "system")]'`
   - Expected (Phase D-4 + E-4 minimum) URIs:
     - `system:/contribution-type/contribution-type`
     - `system:/contribution-type/data-type`
     - `system:/contribution-type/sidecar-info`  *(Phase E-4)*
     - `system:/data-type/contribution`
     - `system:/data-type/sidecar-info`  *(Phase E-4)*
   - Fetch each system URI and verify the Body decodes.
   - **System data-type body shape (2026-06-05 Bug 3 fix — CRITICAL probe)**:
     for every `system:/data-type/*` URI, verify the body decodes as a
     `DataTypeBody` envelope: `{format: "yaml" | "json", source: "<raw DSL bytes>"}`.
     If you see the unwrapped DSL directly (`{name: "...", ...}`), the
     loader regressed to pre-fix shape and the hub's Phase 2 schema
     registration will silently fail — flag as KO with reason
     "system data-type body not wrapped in DataTypeBody envelope (Bug 3 regression)".
     Probe: `$BIN sc resources get <system-uri> --out json | jq '.contribution.body | {format, has_source: (.source != null)}'`
     — must show both `format` and `has_source: true`.
4. **Sidecar-local contributions** — verify each well-known type has
   coverage:
   - Skills: count ≥ 1
   - Tasks (if the sidecar publishes tasks): count ≥ 0 (no error)
   - Data-types: count ≥ 1
   - Service interfaces: count ≥ 1

### Section 5 — Skill traversal (referenced tools + sub-skills)

For each skill returned by the `llm` resources-list probe filtered to `--type skill`:

1. Fetch the skill body via the `llm` resources-get probe for `<skill-uri>`
2. Inspect the Body — it's typically a Markdown document with
   YAML frontmatter
3. From the frontmatter / body, extract:
   - Referenced **tools** (look for `Tool:` lines, ` ``backtick code blocks``, or YAML `tools:` arrays)
   - Referenced **sub-skills** (look for URIs of the form
     `<provider>:/skill/<name>` or `<provider>:<name>` shorthand)
4. For each referenced tool, verify it's in the `llm` tools-list probe
   (`tools/list` method) — KO if missing
5. For each referenced sub-skill URI, fetch via the `llm` resources-get
   probe for `<uri>` — KO if not found

Heuristic: if the skill body has ≥ 1 inline backtick reference that
looks like a tool or skill name and you can't find it via list/get,
record it under findings but don't mark KO unless the unresolved
reference is explicit (e.g. `Tool: foo` syntax).

### Section 6 — Data-type publication & resolution

1. List all data-types from the resources-list JSON, via jq:
   `... | jq '[.contributions[] | select(.type == "data-type")]'`
2. For each data-type:
   - Fetch: `$BIN sc resources get <uri>`
   - Verify Body decodes as a YAML/JSON Schema-like object
3. Verify the schema service round-trip works for any one type:
   - `$BIN sc resources get-data-type <name>` (if the sub-verb exists)
   - Otherwise via `llm resources get`

### Section 7 — Interface publication + method-call-slot ↔ interface coupling

**Architectural expectation**: every service instance exposed as a
method-call slot MUST also be published as an `interface`-type Contribution
(or `data-type` Contribution with semantic role "interface"). The
interface body describes the methods with their input/output type
references — making the slot fully self-describing through the
catalog without needing a live `ktsp describe` call. Missing this
linkage is a structural gap, not a soft warning.

Probe order:

1. **Enumerate method-call slots** the binary publishes:
   `$BIN sc ktsp describe --out json | jq '[.ktsp.slots[] | {name, typeName, methods: [.methods[]?.name]}]'`
2. **List all interface contributions** from the resources-list JSON, via jq:
   `... | jq '[.contributions[] | select(.type == "interface")]'`
   Some binaries publish interfaces as `Type: "data-type"` with a
   naming convention (e.g. `<provider>:/data-type/<ServiceName>Interface`).
   If `Type: "interface"` returns 0, fall back to that pattern and
   note the convention divergence.
3. **For each method-call slot, verify there's a matching interface
   contribution**:
   - Expected URI form: `<provider>:/interface/<slot-name>` or
     `<provider>:/data-type/<slot-name>` (sidecar-specific —
     document which convention the binary uses).
   - If no matching contribution exists → **KO** with reason
     "method-call slot `<name>` has no published interface contribution".
4. **For each interface contribution, fetch its body**:
   - Verify it decodes as `meta.ServiceDef`-shaped JSON (or its
     successor shape after Phase C lands).
   - Walk `methods` — verify each method has identifiable
     `InputType` and `OutputType` references (URI form expected).
   - For each `InputType` / `OutputType` URI, fetch via
     `$BIN sc resources get <ref-uri>`. **KO if any ref
     doesn't resolve.**
5. **Cross-reference roundtrip**:
   - Every method-call slot should have one published interface ✅
   - Every interface should reference at least one method's I/O
     types and those types should resolve as data-type contributions ✅
   - Count unresolved references explicitly — these are broken
     architectural links the catalog can't repair.

Report the **expected vs actual** count in the report's Section 7
table — even if the binary publishes 0 interfaces today, the
expected count = `<number of method-call slots>` and every missing
interface is recorded as a KO row.

**Expected behaviour for `WithLocal()` services with 0 declared methods (NOT KO):**

A service published via `WithLocal()` whose Go interface declares ONLY
non-canonical method shapes (`func(name string) (X, error)`,
`func() []string`, etc. — methods that don't match the publishable
RPC signature `func(ctx context.Context, in T) (U, error)`) is a
**pure in-process accessor** and its `ServiceDef.Methods` map is
INTENTIONALLY empty. SkillManager is the reference example
(`internal/extensions/skills/api/skill_manager.go`)
— after the 2026-05-24 RPC drop it publishes ZERO methods but stays
declared so `application.UseService[SkillManager](ctx)` resolves
in-process.

Decision rule for Section 7's "Methods declared" column:
- If the Go interface has at least one `func(ctx, in T) (U, error)`
  signature AND the produced ServiceDef body shows `Methods: {}` →
  **KO** (the producer dropped methods that should be RPC-publishable).
- If the Go interface has NO canonical RPC signatures (purely
  in-process accessors) AND the body shows `Methods: {}` →
  ✅ EXPECTED. Record as "0 methods (in-process accessor — `WithLocal()`)".

You can't tell from the contribution body alone which case applies —
the Go interface lives in source. When unsure, mark the row as
`ℹ️ NEEDS SOURCE LOOKUP` and reference the service's `api/` file
path so a reviewer can confirm.

### Section 8 — Cross-surface service/method media matrix

This is the canonical "where can I call this?" inventory. Build it
from the data you've already gathered:

- **Services**: from the operator-surface resources-list probe filtered to
  `--type service --out json`
  (one row per service interface contribution, plus any `Provider="local"`
  ones surfaced via `--include-local`). Each service's methods come from
  the contribution Body (a `meta.ServiceDef` JSON).
- **Methods**: walk each service's `Methods` map. Each method needs ONE
  row.
- **Surface availability** is derived per method:
  - **CLI**: present when the method has a matching `sc <slot> <method-kebab>`
    OR a `<domain> <method-kebab>` sub-command. Probe by running
    `$BIN sc <slot> --help` (look for the method-kebab name in the
    subcommand list) — if absent, the binary published the slot with
    `WithKTSPSlotTags({"cli":"false"})`.
  - **LLM**: present when the `llm` tools-list JSON (`tools/list`, `--out json`),
    piped to `jq '.[] | .uri'`, contains `<provider>:<slot>/<method-kebab>`.
  - **method-call stdio**: present when the method is callable via the JSON-RPC
    request `{"method":"<slot>/<method-kebab>"}` and produces a non-error
    result. Probed in Section 3.5 for read-only methods; for mutating
    methods, mark "available NOT TESTED".
  - **HTTP**: available when the binary publishes via `WithHTTPServer(...)`
    (visible in `sc ktsp describe --json` as a capability flag).
    **NOT TESTED** by this skill; record as "available NOT TESTED" or
    "not published".
  - **MCP**: bridge in ktw-hub. Mark as "available NOT TESTED" if
    the operator-surface resources-list probe (`--type service`) shows a
    `<provider>:/service/<slot>`
    entry (the bridge will publish it); mark "not published" for local
    services.
- **Read-only flag**: from the method's `Side` / `ReadOnly` tag in
  the ServiceDef Body, OR by name heuristic (`List`, `Get`, `Describe`,
  `Types`, `Info`, `Manifest`, `Check`, `Dirs` are read-only; `Run`,
  `Set`, `Create`, `Delete`, `Update`, `Install`, `Reload`, `Apply`
  are mutating). Document the heuristic used in the report's findings.

Read-only methods get exercised through every available surface and
their results compared for parity. Mutating methods get recorded as
"available NOT TESTED" — the skill never invokes them per the strict
constraints.

The matrix is emitted in the report's Section 8 (see report skeleton
below). Use ✅ for "available + tested OK", ⚠️ for "available NOT
TESTED" (mutating), ❌ for "available but failed", `—` for "not
available on this surface".

## Report format

Write the report at `test_results/<BIN_NAME>-<YYYYMMDD-HHMMSS>.md`.
Create the directory with `mkdir -p test_results` if needed. Use the
Bash tool for the mkdir; use the Write tool for the report itself.

### Report skeleton

```markdown
# Sidecar Test Report — <BIN_NAME>

- **Binary**: `<full BIN path>`
- **Tested**: `<ISO-8601 timestamp>`
- **Version**: `<output of $BIN version>`
- **Tester**: Claude (sidecar-testing skill)
- **Total**: `<X> OK / <Y> tested` — **<PASSED / FAILED>**

## Section 1 — Top-level CLI discovery

### Commands tested

| Command | Expected | Result | Notes |
|---|---|---|---|
| `$BIN --help` | Rich text help | ✅ OK | … |
| `$BIN version` | Semver line | ✅ OK | … |
| ... | | | |

### Findings

- …

## Section 2 — Operator surface (sc group)

[same skeleton: commands tested table + findings]

## Section 3 — LLM-driving surface (llm group)

[same skeleton]

## Section 3.5 — method-call stdio bus surface

### Probes tested

| Request | Expected | Result | Notes |
|---|---|---|---|
| `resources/types` `{}` | string array matching CLI | ✅ / ❌ | … |
| `resources/list` `{}` | bus-visible Contributions; **no `Provider:"local"`** | ✅ / ❌ | … |
| `resources/list` `{"includeLocal":true}` | full set incl. local | ✅ / ❌ | parity with CLI |
| `resources/list` `{"filter":{"types":["skill"]}}` | filter respected | ✅ / ❌ | … |
| `resources/get` `{"uri":"…"}` | non-empty Body | ✅ / ❌ | … |
| `resources/manifest` `{}` | full descriptor | ✅ / ❌ | … |
| `sidecar/info` `{}` (when Sidecar is bus-exposed) | identity + services + skills | ✅ / ❌ | … |
| `sidecar/info` `{}` (when Sidecar is `WithLocal()`) | RPC error `Slot not found: sidecar` + identity available via `<provider>:/sidecar-info/default` | ℹ️ EXPECTED / ❌ | not a KO — see WithLocal branch rule in Section 3.5 |

### CLI ↔ stdio parity (REGRESSION GATE)

| Pair | Match | Notes |
|---|---|---|
| operator resources-list (`--type task`) ↔ `resources/list {filter:{types:[task]}}` (stdio) | ✅ / ❌ | id sets identical |
| call `<p>:resources/list` tool with `{}` (llm) ↔ stdio equivalent | ✅ / ❌ | result arrays identical |

### Local-scope bus boundary

| Probe | Expected | Result |
|---|---|---|
| `Provider=="local"` entries via stdio default | absent | ✅ / ❌ |
| `Provider=="local"` entries via stdio `{includeLocal:true}` | present | ✅ / ❌ |
| `Provider=="local"` entries via local CLI | present | ✅ / ❌ |

### Findings

- …

## Section 4 — Contribution coverage

### Full resource inventory (mandatory)

The report MUST list **every resource URI** the binary publishes,
grouped by type, with the resolution status. This is the
catalog-completeness snapshot — readers should be able to scan
this section and immediately see what's there AND what's missing.

For each well-known type below, INCLUDE THE TABLE EVEN WHEN THE
COUNT IS 0 (an empty table with a "MISSING — expected ≥ N" note
in the count column is a KO signal, not a row to skip):

Every row carries a **Scope** column: `system` (bootstrap), `<binary-name>`
(bus-visible sidecar entry), or `local` (binary-private, CLI-only).

#### Tools (callable verbs) — URI form `<provider>:<slot>/<method>`

| URI | Scope | Description | Schema available | Notes |
|---|---|---|---|---|
| `<provider>:<slot>/<method>` | <scope> | one-line description | ✅/❌ | … |
| ... | | | | |
| **Total** | | **N tools** | | |

#### Skills — URI form `<provider>:/skill/<id>`

| URI | Scope | Name (from Metadata) | Body resolves | Notes |
|---|---|---|---|---|
| `<provider>:/skill/<id>` | <scope> | display name | ✅/❌ | … |
| ... | | | | |
| **Total** | | **N skills** | | |

#### Tasks (if applicable) — URI form `<provider>:/task/<id>`

| URI | Scope | Name | Body resolves | Notes |
|---|---|---|---|---|
| ... | | | | |
| **Total** | | **N tasks** (or 0 — KO if expected) | | |

#### Data-types — URI form `<provider>:/data-type/<id>`

| URI | Scope | Schema ref | Body resolves | Notes |
|---|---|---|---|---|
| ... | | | | |
| **Total** | | **N data-types** | | |

#### Interfaces (services) — URI form `<provider>:/interface/<id>` or `<provider>:/service/<id>`

| URI | Scope | Methods declared | Body resolves | Notes |
|---|---|---|---|---|
| ... | | | | |
| **Total** | | **N interfaces** (expected ≥ <method-call-slot-count> per Section 7) | | |

#### Sidecar identity — URI form `<provider>:/sidecar-info/default`

| URI | Scope | Body resolves | Notes |
|---|---|---|---|
| `<provider>:/sidecar-info/default` | <binary-name> | ✅/❌ | … |

#### Other types (config-file, env, reference, …)

| Type | Count | Sample URIs | Notes |
|---|---|---|---|
| config-file | N | … | |
| env | N | … | |
| reference | N | … | |

### System contributions

| URI | Resolves | Body decodes | Envelope wrap | Notes |
|---|---|---|---|---|
| `system:/contribution-type/contribution-type` | ✅/❌ | ✅/❌ | n/a (not a data-type) | … |
| `system:/contribution-type/data-type` | ✅/❌ | ✅/❌ | n/a | … |
| `system:/contribution-type/sidecar-info` | ✅/❌ | ✅/❌ | n/a | Phase E-4 addition |
| `system:/data-type/contribution` | ✅/❌ | ✅/❌ | `{format, source}` ✅/❌ | Bug 3 envelope-wrap probe |
| `system:/data-type/sidecar-info` | ✅/❌ | ✅/❌ | `{format, source}` ✅/❌ | Phase E-4 + Bug 3 |
| ... (any other system URIs the loader emits) | | | | |

**Mandatory**: every expected system URI gets a row, even if it's
absent (row marked ❌ KO). An empty system section is a Phase D-4
gap that consumers need to see explicitly.

### Sidecar-local contributions per type — summary

| Type | Count | All decode | Notes |
|---|---|---|---|
| skill | N | ✅/❌ | … |
| data-type | N | ✅/❌ | … |
| interface | N | ✅/❌ | match against method-call slot count |
| task | N | ✅/❌ | … |
| (other types) | N | ✅/❌ | … |

## Section 5 — Skill traversal

| Skill URI | Body decodes | Referenced tools resolved | Referenced sub-skills resolved | Notes |
|---|---|---|---|---|

## Section 6 — Data-type publication

| URI | Body decodes | Schema lookup | Notes |
|---|---|---|---|

## Section 7 — Interface publication & method-call-slot coupling

### method-call slot ↔ interface contribution matrix (mandatory)

For each method-call slot exposed by the binary, record whether a matching
interface contribution exists:

| method-call slot | Methods (count) | Expected interface URI | Interface published | Result |
|---|---|---|---|---|
| `<slot-typeName>` | N | `<provider>:/interface/<slot-typeName>` (or convention) | ✅ / ❌ | ✅ OK / ❌ KO |
| ... | | | | |

Each row where the interface is missing → **KO**, recorded with the
reason "method-call slot `<name>` has no published interface contribution".

### Interface body cross-reference (per published interface)

| Interface URI | Methods documented | InputType refs resolve | OutputType refs resolve | Notes |
|---|---|---|---|---|
| `<provider>:/interface/<id>` | N | ✅ all / ❌ M unresolved | ✅ all / ❌ M unresolved | … |
| ... | | | | |

Count unresolved type references in the totals. A method whose
`InputType` or `OutputType` URI doesn't fetch is a broken
architectural link.

### Coverage summary

- method-call slots: **N**
- Interface contributions: **M**
- Coupling: **N == M** ✅ / **N != M (gap = N-M)** ❌
- Unresolved method-type refs: **K** (target = 0)

## Section 8 — Cross-surface service/method media matrix

For EVERY published method on EVERY service (including local-scope
services), record one row.

Legend:
- ✅ = surface available + tested OK (read-only method)
- ⚠️ = surface available + NOT TESTED (mutating method, per skill's
  read-only constraint)
- ❌ = surface available but failed/empty
- `—` = surface not available

| Provider | Slot | Method | Read-only? | CLI | LLM | method-call stdio | HTTP | MCP |
|---|---|---|---|---|---|---|---|---|
| `ktw-example` | `contributions` | `list` | yes | ✅ | ✅ | ✅ | ⚠️ avail. NOT TESTED | ⚠️ avail. NOT TESTED |
| `ktw-example` | `contributions` | `get` | yes | ✅ | ✅ | ✅ | ⚠️ | ⚠️ |
| `ktw-example` | `config` | `set` | no | ⚠️ avail. NOT TESTED | ⚠️ | ⚠️ | ⚠️ | ⚠️ |
| ... | | | | | | | | |

Per-row notes (optional, only when the row has ❌ or a worth-mentioning
quirk):

- … (e.g. "config/set CLI hidden behind --confirm flag")

### Matrix coverage summary

- Total methods: **N**
- Read-only methods: **R** (tested across CLI + LLM + method-call stdio)
- Mutating methods: **N - R** (recorded as "available NOT TESTED")
- Methods available on all three tested surfaces: **A** (target = N)
- Methods missing from CLI: **C**, from LLM: **L**, from method-call stdio: **K**
- HTTP surface published? **yes / no** (one capability flag for the
  whole binary)
- MCP surface published? **yes / no** (derived from service contribution
  presence)

## Cross-cutting Remarks

### CLI Quality

- **Intuitive command names?** [yes/partly/no — with examples]
- **Overlapping commands?** [list pairs where two paths reach the same
  data, e.g. the operator-surface vs the `llm` group both exposing
  resources-list]
- **Glaring gaps?** [operations needed but absent]
- **Short alias consistency?** [is `sc` always the short alias? are
  `tls`/`rsc` used uniformly?]

### Flag / Option Quality

- **`--out json` honored everywhere on rich-default commands?**
- **`--out rich` honored everywhere on JSON-default commands?**
- **Required flags clearly marked?**
- **Default values sensible?**
- **Any flag/positional-arg overlap that's confusing?**

### LLM-driven Usability

- **Is the `llm` group complete enough for an LLM agent to drive every
  public function without falling back to `sc`?**
- **Are there capabilities reachable ONLY via `sc` that an LLM would
  want?** [list them]
- **Is `llm info` comprehensive enough for one-shot introspection?**
  [does it cover identity, conventions, tools, resources, skills,
  configuration; does it work offline]
- **JSON shape stability**: are tool / resource JSON shapes consistent
  across `list` and `get` for the same entry?

### Information Completeness

- **All contributions reachable?** [count from the operator-surface
  resources-list probe matches sum across `--type=X` filters?]
- **System contributions present?** [Phase D-4 minimum: 3 URIs]
- **Interface→data-type references resolvable?** [count of unresolved
  refs across all interfaces]
- **Skill→tool/sub-skill references resolvable?** [count of unresolved
  refs across all skills]

### Summary table

| Battery section | OK | KO | Notes |
|---|---|---|---|
| 1. Top-level CLI | … | … | |
| 2. Operator surface | … | … | |
| 3. LLM surface | … | … | |
| 3.5 method-call stdio bus surface | … | … | parity + local-scope boundary |
| 4. Contribution coverage | … | … | |
| 5. Skill traversal | … | … | |
| 6. Data-type publication | … | … | |
| 7. Interface cross-reference | … | … | |
| 8. Cross-surface matrix | … | … | read-only methods tested; mutating recorded |

**Total: X OK / Y tested → PASSED / FAILED**

### Top 3 recommended fixes (if FAILED)

1. …
2. …
3. …
```

## Operational rules while running

1. Run each command via Bash with a 30s timeout. Capture combined
   stdout+stderr with `2>&1`.
2. For JSON-default commands, validate with `jq -e .` (exit code 0
   = syntactically valid).
3. For rich-default commands, treat a non-empty stdout with at
   least one line containing letters as OK; treat empty output as KO.
4. Don't memoize captured outputs in repo files. Hold them in
   memory while writing the report.
5. When a command takes a URI argument, prefer URIs you discovered
   from a prior `list` call. Don't hand-construct URIs unless the
   convention is documented in the system contributions.
6. If a probe is non-deterministic (e.g. depends on credentials),
   record the dependency in the report's findings and mark the
   probe as SKIPPED (counts as neither OK nor KO).
7. If the binary itself errors in a way that aborts the test (e.g.,
   refuses to start), stop and write a minimal report explaining
   the abort condition.

## Self-validation

Before reporting PASSED, sanity-check the report:

- Every section has a Commands tested table with at least 1 row
- Every cross-cutting Remarks subsection has at least 1 bullet
- The Summary table totals match the per-section pass counts
- The result verdict (PASSED / FAILED) matches the totals
- The report timestamp is in the filename AND in the header

## Reference binary

Validate the skill end-to-end against a built domain sidecar
(`./bin/ktw-example.exe` stands for it). A domain sidecar typically
exposes:

- Top-level groups: `query`, `search`, `graph-walk`, `run-report`,
  `to-diagram`, `introspect`, `link-info`, `list`, `sidecar` (alias
  `sc`), `llm`
- Standard `sc resources {list,get,types,manifest}` surface
- Standard `llm {info,manifest,tools,resources}` surface
- ≥ 6 skills, ≥ 8 tools, ≥ 1 service interface

A successful run against this binary should produce a PASSED report
with ≥ 40 commands+probes tested (Sections 1–3 contribute ~20 CLI/LLM
calls, Section 3.5 adds ~10 stdio JSON-RPC probes, Section 8's matrix
rows count once each).

### Phase E §5b regression gate

Sections 3.5 + 8 exist specifically to catch the kind of bug introduced
by Phase E §5b (`resources/list` over stdio returned `[]` because
the request ctx wasn't derived from `cmd.Context()`). When you run
this skill after a build:

- If Section 3.5's "CLI ↔ stdio parity" rows fail with empty stdio
  results, the build has regressed `StdioServe` ctx propagation
  (`internal/kts/protocol/ktsp/jsonrpc/session.go:StdioServe`).
- If Section 3.5's "Local-scope bus boundary" shows local entries
  leaking onto the bus, the `IsBusVisible` filter in
  `manifest/internal/core/service.go::List` regressed.

Both should be flagged in the Top-3 fixes block, not buried in
findings.

## Out of scope

- The sidecar's own domain operations (`query`, `search`, etc.) when they
  require live credentials. Document them in Section 1 as observed
  but don't invoke them.
- Schema validation of bodies against their data-type ASTs — that
  requires the SchemaService running. Mark schema validation as
  SKIPPED if the sidecar isn't connected to a hub.
- Performance / load testing. This skill is correctness-focused.
