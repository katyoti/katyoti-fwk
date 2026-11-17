# ktw-agents Makefile
# Usage: make [target]

BINDIR    ?= bin
BENCHDIR  := benchmarks

# --- Target platform -------------------------------------------------------
# Defaults build for the host (Windows). A cross-build overrides all four:
#
#   make build CROSS="GOOS=darwin GOARCH=arm64" EXT= \
#              BUN_TARGET=bun-darwin-arm64 BINDIR=dist/darwin-arm64
#
# Prefer the dist-* targets below, which set them for you.
#
# CROSS is an env prefix, empty for a host build. EXT is the executable
# suffix — Windows needs `.exe`, macOS and Linux need none, and the Go-side
# binary discovery (sidecar/client/lookup.go) already keys off runtime.GOOS,
# so an extension-less binary is found correctly on macOS.
CROSS      ?=
EXT        ?= .exe
BUN_TARGET ?= bun

# The apps the open-source export publishes; the private ones are added in the fence
# (an oss:private fence is cut by the export, tools/oss-export).
APPS      := ktw-agents ktw-notify ktw-claude ktw-utils ktw-docs ktw-sync ktw-kb ktw-code ktw-gocode ktw-univers ktw-demo ktw-hub ktw-weather ktw-compose ktw-methods katyoti ktw-host

# Version stamped into every binary's `cmd.version` via -ldflags at build time
# (each app has `var version` in app/<name>/cmd/root.go).
#
# Default (VERSION=dev): the binary self-reports its git commit via Go's VCS
# build info — `appcore.ResolveVersion` derives it, so no build-time git call
# is needed. `make katyoti` → e.g. `9a8edf4b-dirty` for free.
#
# For an explicit / semver stamp, pass VERSION (your shell resolves it):
#   make katyoti VERSION=1.2.3
#   make katyoti VERSION=$(git describe --tags --always --dirty)
# A non-`dev` VERSION always wins over the VCS-derived default.
#
# GOTCHA: gomake does NOT export make variables into a recipe's environment.
# A recipe that interpolates $(VERSION) into its command line is fine; a recipe
# that hands off to a SCRIPT must pass it explicitly as an env prefix —
# `VERSION=$(VERSION) sh scripts/foo.sh`. Without that the script sees nothing
# and silently falls back to `dev`, producing an unversioned release.
VERSION   ?= dev
MODULE    := github.com/katyoti/katyoti-fwk

# Strip the symbol table (-s) + DWARF (-w) from every binary by default. This
# cuts ~30% off the file size (measured: ktw-hub 34MB→24MB), which directly
# shortens on-access antivirus scans (the dominant cold-start cost on machines
# with Webroot/Bitdefender/etc.) AND shrinks downloads. Go runtime panic traces
# are UNAFFECTED — they read the pclntab, not the stripped symbol table.
# Set STRIP=0 to keep symbols for a debugger (delve needs DWARF): `make X STRIP=0`.
STRIP     ?= 1
ifeq ($(STRIP),1)
STRIP_LDFLAGS := -s -w
endif


.PHONY: hooks solution-update tasks-l1 tasks-l2 all build clean test test-quick test-affected test-full vet vet-mac lint check import-discipline uri-discipline extension-layout acceptance katyoti-e2e e2e e2e-binaries e2e-warm frontend-typecheck cover help ktw-gui-mac $(APPS) ktw-gui ktw-gui-debug ktw-display ktw-mcp ktw-test

# Default: build all binaries
all: build

# Build all Go binaries + ktw-display (TypeScript via bun) into bin/.
# ktw-gui is excluded — Wails has its own build flow (target below).
build: hooks $(APPS) ktw-display ktw-mcp

# Install the repository's git hooks (.githooks: pre-commit refuses a commit
# that adds a secret) for this clone and its worktrees.
# Idempotent; nothing outside a git checkout (an archive copy).
hooks:
	@if [ -d .githooks ] && git rev-parse --git-dir >/dev/null 2>&1; then git config core.hooksPath .githooks; fi

$(APPS):
	$(CROSS) go build -ldflags "$(STRIP_LDFLAGS) -X $(MODULE)/app/$@/cmd.version=$(VERSION)" -o $(BINDIR)/$@$(EXT) ./app/$@/

# Build ktw-mcp — the KaTyoti MCP server used like the command line
# (modules/katyoti/claude-code.md): one tool per program beside it. Claude Code
# keeps it running for a session, so it is built aside and put in place by
# place-binary.sh (a running copy is renamed, never overwritten). Beside it go
# the channel's allow-list (MCP_CHANNEL) and the dev plugin folder, loaded with
# `claude --plugin-dir bin/ktw-dev-plugin`.
MCP_CHANNEL ?= dev
ktw-mcp:
	$(CROSS) go build -ldflags "$(STRIP_LDFLAGS) -X $(MODULE)/app/ktw-mcp/cmd.version=$(VERSION)" -o $(BINDIR)/ktw-mcp.new$(EXT) ./app/ktw-mcp/
	sh app/ktw-display/scripts/place-binary.sh $(BINDIR)/ktw-mcp.new$(EXT) $(BINDIR)/ktw-mcp$(EXT)
	cp app/ktw-mcp/channels/$(MCP_CHANNEL).allow.yaml $(BINDIR)/ktw-mcp.allow.yaml
	rm -rf $(BINDIR)/ktw-dev-plugin
	mkdir -p $(BINDIR)/ktw-dev-plugin
	cp -r app/ktw-mcp/plugin/. $(BINDIR)/ktw-dev-plugin/
	sed -i "s/@EXT@/$(EXT)/" $(BINDIR)/ktw-dev-plugin/.mcp.json

# Build ktw-test — the dev test harness binary (HTTP spawn server +
# the `test` subcommand that runs our integration suites). Not in
# $(APPS): it's a dev tool, not a shipped sidecar. Build explicitly:
# `make ktw-test`.
ktw-test:
	$(CROSS) go build -ldflags "$(STRIP_LDFLAGS) -X $(MODULE)/app/ktw-test/cmd.version=$(VERSION)" -o $(BINDIR)/ktw-test$(EXT) ./app/ktw-test/

# Build ktw-gui (Wails app — requires wails CLI).
# `-windowsconsole` keeps a console window attached: (a) gives the dev
# a place to see panics + Wails-runtime warnings; (b) makes child
# processes spawned by ktw-gui (ktw-hub, per-universe ktw-agents)
# inherit the console handle instead of each opening a new console
# window. Application logs (Go std log + Wails LogPrint) go ONLY to
# <DataDir>/logs/ktw-gui-<timestamp>.log thanks to log.SetOutput in
# main.go — they MUST NOT print to the console.
# NOTE: `-skipembedcreate` is REQUIRED with Go >= 1.25. Wails v2.11.0 pins
# golang.org/x/tools v0.30.0, whose go/packages cannot read the export data
# emitted by newer Go toolchains; the embed-dir pre-pass (which only mkdir's
# missing //go:embed targets) dies with
#   internal error: package "fmt" without types was imported from ...
# frontend/dist is gitignored: a fresh checkout has none, and
# scripts/build-cross.sh makes the embed target before wails (the release
# builds from a clean checkout); here, the folder is left by earlier builds.
ktw-gui:
	cd app/ktw-gui && wails build -windowsconsole -skipembedcreate -ldflags "$(STRIP_LDFLAGS)"
	cp app/ktw-gui/build/bin/ktw-gui.exe $(BINDIR)/ktw-gui.exe


# Build ktw-gui in debug mode — enables DevTools (Ctrl+Shift+I / right-click
# Inspect), the JS console, and the full WebView inspector. Use for live
# diagnostics of the KTSP browser, the schema viewer, the Wails<->Go bridge,
# etc. The output binary still lands at bin/ktw-gui.exe.
ktw-gui-debug:
	cd app/ktw-gui && wails build -debug -devtools -windowsconsole -skipembedcreate
	cp app/ktw-gui/build/bin/ktw-gui.exe $(BINDIR)/ktw-gui.exe

# Build ktw-display — the TypeScript sidecar that serves widget URLs +
# the in-iframe runtime bundle. Compiled to a single self-contained exe
# via `bun build --compile`, lands alongside every Go sidecar in bin/ so
# the standard `ScanLocal` discovery picks it up like any other ktw-*
# binary. Requires `bun` in PATH.
#
# Pipeline:
#   1. prebuild — compiles the React runtime bundle into
#      `src/runtime/_compiled/runtime.{js,css}` so `bun build` can text-
#      import it. Runtime-side `Bun.build()` does NOT work in compiled
#      binaries (no source-file access at runtime).
#   2. compile — produces bin/ktw-display.exe with the runtime bundle
#      + the small drawio viewer (~3.7 MB) embedded.
#   3. assets-copy — copies vendor/drawio/editor/ (50 MB webapp, can't
#      practically be embedded) into bin/ktw-display-assets/drawio/
#      editor/. ktw-display reads from this sibling dir at runtime when
#      the iframe asks for /widget/vendor/drawio/editor/... — the read-
#      only viewer continues to come from the embedded blob.
#   bun compiles to ktw-display.new.exe and scripts/place-binary.sh puts it
#   in place: Windows refuses to overwrite the RUNNING exe (a universe serving
#   widgets keeps it open — bun's own move fails with EPERM), so a running
#   copy is renamed aside first and reaped by a later build once it exits.
# $(BUN_TARGET) selects the embedded Bun runtime: `bun` for the host, or an
# explicit `bun-<os>-<arch>` (bun-darwin-arm64, bun-darwin-x64, bun-linux-x64)
# to cross-compile. Bun downloads the target runtime on first use.
ktw-display:
	cd app/ktw-display && bun run scripts/build-runtime-bundle.ts && bun build src/index.ts --target $(BUN_TARGET) --outfile ../../$(BINDIR)/ktw-display.new$(EXT) --compile --define process.env.KTW_DISPLAY_VERSION='"$(VERSION)"'
	sh app/ktw-display/scripts/place-binary.sh $(BINDIR)/ktw-display.new$(EXT) $(BINDIR)/ktw-display$(EXT)
	sh app/ktw-display/scripts/copy-editor-assets.sh



# Standalone ktw-gui for macOS — one universal (arm64 + amd64) .app bundle,
# without rebuilding the sidecars. macOS host only; needs Xcode CLI tools and
# the wails CLI. `make dist-mac` on a Mac already covers this.
ktw-gui-mac:
	cd app/ktw-gui && wails build -platform darwin/universal -skipembedcreate

# Regenerate the GUI's TYPED KTSP clients from the live descriptor.
#
# The GUI reaches services through ONE generic Wails dispatcher; the typing is
# generated from the same descriptor the wire is built from, so it cannot drift.
# Run this after publishing a service method — no Go change in ktw-gui is needed.
# See rule management-plane-hub-authority invariant 5.
gen-ktsp-client: ktw-univers ktw-utils
	bash scripts/gen-ktsp-client.sh

# The test targets run without fsync: KTW_ATOMICIO_NO_SYNC=1
# (scripts/test-env.sh says why; test-quick, test-affected and test-full set it
# through their scripts); internal/core/atomicio's own tests clear it.
# They pass go test -timeout 30m, as the scripts do (KTW_TEST_TIMEOUT): slow is
# named, not failed.
# Run all tests, under the guard that fails the run when one wrote the
# person's installations or registries
test:
	KTW_ATOMICIO_NO_SYNC=1 bash scripts/installation-guard.sh go test -timeout 30m ./...

# The two test lanes. The quick lane, every change:
# test-affected runs the packages of what changed (against HEAD, SINCE=<ref>,
# or PATHS="<files>") and their importers; test-quick runs PKGS (default
# ./...); both go test -short -p 2, the cache kept - the heavy tests listed in
# internal/test/long skip. test-full, the final gate or the night, one at a
# time, on a clean snapshot of a commit (COMMIT=<ref>, default HEAD), never
# the shared working tree: its bin/ built there, every Go test again, two
# packages at a time, under the installation guard, then the frontend suites
# one at a time; its report, naming the commit, is kept in bin/test-runs/.
# All end on the slowest packages.
PKGS ?= ./...
test-quick:
	bash scripts/test-quick.sh $(PKGS)

test-affected:
	bash scripts/test-affected.sh --since "$(SINCE)" --paths $(PATHS)

test-full:
	bash scripts/test-full.sh $(if $(COMMIT),--commit $(COMMIT))


# Tests + a per-function coverage report.
cover:
	KTW_ATOMICIO_NO_SYNC=1 go test -timeout 30m -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out



# `katyoti solution update` end to end, in a world of its own: a
# marketplace offering the solution demo at v1 (data version 1), v2 (the first
# migration step) and v3 (a step that fails); v1 installed, updated to v2 (data
# migrated, objects kept), then a failed update to v3 rolled back to v2.
# Runs under $KTW_TEST_ROOT/solution-update (else a temporary folder).
solution-update:
	go test -tags solution_update -count=1 -v -timeout 40m ./internal/test/solutionupdate/

# The task runtime's test ladder (modules/tasks/tests/README.md).
# L1: the in-process suites of the task engine and its consumers (~3 min).
# L1-race: the same, under the race detector (Linux, through WSL on Windows).
# L2: real processes in a sandbox universe — kill -9, clean stop, a quit CLI run,
# a second serving process, a hub restart. Build-tagged; ~4 minutes.
TASKS_L1_PKGS = ./internal/extensions/task/... ./internal/extensions/comms/... ./internal/extensions/agent/... ./internal/extensions/orchestration/... ./internal/kts/local/...
tasks-l1:
	go test -count=1 -timeout 15m $(TASKS_L1_PKGS)
tasks-l2:
	go test -tags tasks_l2 -count=1 -v -timeout 15m ./internal/extensions/task/internal/l2test/

# katyoti marketplace end-to-end harness — builds the katyoti (consumer) +
# ktw-compose (publisher) binaries, then drives the verified CLI flows:
# publisher chain (keygen/build/sign/publish+verify), signed-trust
# (pinned add/verify/tamper-refuse), content install (+sha256 reject/uninstall),
# and release (install+wire → snapshot/immutability → bootstrap). Build-tagged
# so it stays out of the default `go test ./...`. Runtime ~45s (mostly the build).
katyoti-e2e:
	go test -tags katyoti_e2e ./internal/extensions/marketplace/e2etest/... -count=1 -v

# ktw-gocode v1 acceptance test — drives the full chain end-to-end
# against the throwaway gocode-acceptance fixture and asserts all 6
# canonical personas dispatch successfully (specifier → architect →
# coder → tester → reviewer → documenter, status=running with real
# task IDs). Tier state + module health + lint are also asserted.
#
# Requires:
#   - bin/ktw-agents.exe built (`make ktw-agents` first)
#   - .katyoti/.env at repo root with provider API keys (optional;
#     the test downgrades gracefully when absent)
#
# Runtime: ~1s. Per the §15.9 LOCKED decision, runs against the
# purpose-built throwaway module — never against production code.
acceptance:
	KTW_ACCEPTANCE=1 go test ./internal/test-modules/gocode-acceptance/ -count=1 -v

# Run all tests under the race detector. Requires CGO_ENABLED=1 + a C
# compiler (gcc / clang / msvc). Exits non-zero if the race detector reports
# any data race in the concurrent code paths (Executor loop, Channel/Inbox/
# Spawner, OrphanDetector goroutine, MailboxManager rate-limiter mutex,
# recorder subscriptions, etc.).
#
# Local invocation: `make test-race`. Slower than `make test` (~3-5×).
# CI integration: see `ci-add-race-detector-step` task — on-request, not
# every commit (per quality-tooling.md "When to run the gates").
test-race:
	KTW_ATOMICIO_NO_SYNC=1 CGO_ENABLED=1 go test -race -count=1 -timeout 30m ./...


# Run go vet
vet:
	go vet ./...

# Catches macOS-only build breakage without leaving the host — the platform
# splits (syscall, desktop notifications) only fail to compile under GOOS=darwin.
vet-mac:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go vet ./...

# Run golangci-lint (if installed)
lint:
	golangci-lint run ./...

# Enforce every import boundary declared in tools/import-discipline/rules.yaml
# (NATS client scope, embedded-hub scope, the schema parser boundary, extension
# internals). Rules are data; adding one is editing YAML. Cross-platform.
import-discipline:
	@go run ./tools/import-discipline

# Enforce rule that the obsolete `slot://` URI form (decided in 6U,
# superseded 2026-05-23) no longer appears anywhere in code. Tool URIs
# use the unified `<provider>:<slot>[(<instance>)]/<method>` form.
uri-discipline:
	@go run ./tools/uri-discipline

# Refuse a stale extension-package inventory: modules/framework/_generated/extension-layout.md
# is written by `go run ./tools/extension-layout --write` and read against
# modules/framework/extension-layout.md.
extension-layout:
	@go run ./tools/extension-layout --check
# Lint every skill and task of every sidecar executable in bin/ against each
# other: every provider, resource, slot and method a body names must be
# published by some binary. Runs on compiled executables only — a sidecar built
# in another repository is linted the same way — through ktw-compose. A
# sidecar's OWN content is linted at its build by its census; this is the half
# no single sidecar can check about itself.
lint-universe:
	@./bin/ktw-compose$(EXT) lint-universe --bin-dir bin --out json

# The binaries the cross-process E2E suite needs. internal/test/catalog spawns
# a real hub and real sidecars; without these on disk it SKIPS, and a skip
# reads exactly like a pass. `make check` builds them so the suite actually
# runs — which is the whole difference between "the tests are green" and "the
# tests ran".
#
# EVERY Go binary, not the handful one suite happens to name. Several suites
# spawn sidecars (internal/test/catalog, internal/test/demos, the terminal
# smokes) and a PARTIAL rebuild is its own trap: a fresh hub against a
# three-day-old ktw-demo reports "task not found" and looks like a code defect.
# Stale binaries lie in both directions — they hid real failures here and
# invented others — so the only safe input to a cross-process run is a tree
# built from the same commit.
#
# Prerequisites rather than a recipe loop: each app name is already a target
# above, so this reuses the one build rule instead of restating the ldflags.
# ktw-display is excluded — it needs bun, and no Go test spawns it.
e2e-binaries: $(APPS)

# Execute each spawned binary once before the timed run.
#
# On a machine that scans unsigned executables on first execution, a
# just-built binary costs seconds the FIRST time it runs and milliseconds
# after. The suite spawns a hub and six sidecars inside a startup budget, so
# that one-time cost landed as "hub failed to write lockfile" — a scan
# reported as a product failure. Paying it here puts it outside the budget,
# where it is a slow build rather than a red test.
e2e-warm:
	@$(BINDIR)/ktw-hub$(EXT) --version
	@$(BINDIR)/ktw-host$(EXT) --version
	@$(BINDIR)/ktw-agents$(EXT) --version
	@$(BINDIR)/ktw-docs$(EXT) --version
	@$(BINDIR)/ktw-methods$(EXT) --version
	@$(BINDIR)/ktw-compose$(EXT) --version
	@$(BINDIR)/ktw-code$(EXT) --version
	@$(BINDIR)/ktw-gocode$(EXT) --version
	@$(BINDIR)/ktw-demo$(EXT) --version

# Run the cross-process suite for real. KTW_E2E=1 turns its "binary not built"
# skips into failures: once we have built them, a skip means something is
# wrong, not that there is nothing to check.
# Two suites, two questions:
#   internal/test/binaries — run each COMPILED sidecar and judge the manifest it
#       reports, by the same rules the census applies in-process. Between the
#       two sit //go:embed sets, build tags and cmd/ wiring, any of which can
#       make a shipped binary publish something the checked code did not.
#   internal/test/catalog  — what a CONSUMER gets back, through a real hub.
e2e: e2e-binaries e2e-warm
	KTW_E2E=1 go test -count=1 ./internal/test/binaries/... ./internal/test/catalog/...

# Typecheck the ktw-gui frontend against the CURRENT Wails bindings.
#
# Nothing in `check` compiled the TypeScript, so a Go field removed on one side
# broke the GUI build on the other and stayed broken silently: `SchemaV` left
# `ContributionSummary` on 2026-09-17 and the panel reading `summary.schemaV`
# only failed the next time somebody ran `wails build` — a full day later, in a
# release build, with an error that named TypeScript rather than the Go change
# that caused it.
#
# `tsc --noEmit` is the same compile `wails build` runs, minus the bundle.
# Skipped with a notice when node_modules is absent, so a Go-only checkout is
# not forced to install a frontend toolchain.
frontend-typecheck:
	@go run ./tools/frontend-typecheck

# Run vet + test + discipline checks
check: vet import-discipline uri-discipline frontend-typecheck test lint-universe e2e


# Remove build artifacts (host binaries + every cross-built distribution).
# Published marketplaces and their signing keys live with each marketplace's
# definition, outside this repo: a clean never touches them.
clean:
	rm -rf $(BINDIR)/*.exe dist


# --- help ---------------------------------------------------------------------

help:
	@echo "Host build   build clean                -> bin/ (flat, what tests discover)"
	@echo "Git hooks    hooks                      -> core.hooksPath = .githooks (pre-commit refuses a secret)"
	@echo "Checks       test test-affected test-quick test-full test-race cover vet vet-mac lint check bench"
	@echo ""
	@echo "Vars         VERSION CROSS EXT BINDIR BUN_TARGET"


