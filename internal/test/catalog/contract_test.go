// SPDX-License-Identifier: Apache-2.0

// Package catalog holds the headless end-to-end contract test for the
// hub's aggregate CatalogService surface (get / read / list + validation)
// across ALL contribution types.
//
// Why this test exists (and the per-binary `<binary> llm` surface can't
// replace it): each binary's local `llm` surface only sees ITS OWN
// in-process contributions. object-type / edge-type / data-type
// packs are BUNDLED into ktw-agents at compile time and surface only in
// the hub's AGGREGATE catalog once every sidecar is hydrated. Validation
// status (Contribution.Valid + ValidationErrors) is set by the hub's
// Reindex over the aggregate — it does not exist in any single binary's
// view. So the contract for the universe-wide catalog can only be
// asserted against a real running ktw-hub with the main sidecars loaded.
//
// How it calls the hub: in-Go, over the real NATS MessageBus the hub
// exposes. `hubclient.ConnectToHub(ModeReuse)` returns a *HubClient whose
// .MessageBus() feeds `messagebus.NewSender(bus, "local", <universe>)` —
// the exact plumbing app/ktw-gui/internal/supervisor/master.go uses to
// drive `ktw-hub:/CatalogService/*`. No subprocess shelling needed; the
// sender speaks the same wire envelope the GUI does.
//
// Run: go test ./internal/test/catalog/ -run TestCatalogContract -v
// Skips cleanly (via hubharness) when bin/ktw-hub or a listed sidecar
// binary isn't built.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmlocal "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/local"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// universeName is the lowercase universe token written into Univers.yaml.
// It MUST be a valid NATS subject token ([a-z0-9-]{1,63}) — the in-Go
// messagebus.Sender builds slot subjects from it and the subject builders
// PANIC on an invalid token. t.Name() ("TestCatalogContract") is
// PascalCase and would panic, so we pin a deterministic lowercase name
// here and write the manifest ourselves (rather than via
// MakeTempUniverseWithSidecars, which uses t.Name()).
const universeName = "catalog-contract"

// tenantID matches what ktw-gui assigns + what cenv.ResolveTenantUniverse
// defaults to (KTW_TENANT_ID unset → "local"). Publisher (hub) and
// subscriber (this sender) MUST agree on the scope tokens or the slot
// subject never matches.
const tenantID = "local"

// sidecarsToInclude is the set whose aggregate catalog we assert over.
// ktw-agents bundles the methods/code/gocode/compose CWM type packs so
// its manifest carries the substrate (node/item/edge/data-type)
// registrations; ktw-docs carries references and the drawio notations, viewpoints and template libraries; ktw-methods
// carries skills/data-types/personas; ktw-compose carries authoring
// skills.
var sidecarsToInclude = []string{"ktw-agents", "ktw-docs", "ktw-methods", "ktw-compose", "ktw-code", "ktw-gocode"}

// contentTypes are kinds whose Read() MUST return the RAW readable body
// (markdown / file bytes / DSL text / JSON) — NOT the descriptor.
var contentTypes = map[string]bool{
	"skill":           true,
	"task":            true,
	"reference":       true,
	"data-type":       true,
	"notation":        true,
	"viewpoint":       true,
	"drawio-template": true,
	"persona":         true,
	"config-file":     true,
	"config-program":  true,
	"env":             true,
}

// substrateTypes are CWM TYPE REGISTRATIONS whose "body" IS the type
// definition. Read-raw is NOT meaningful for them; the definition is
// retrievable via the index (ListContributions) + GetBody.
var substrateTypes = map[string]bool{
	"object-type": true,
	"edge-type":   true,
}

// perTypeResult is the row we tally + report per contribution type.
type perTypeResult struct {
	Type          string
	Count         int
	ReadOK        int // # whose Read() returned non-empty content (content types only)
	ReadFail      int // # whose Read() returned empty/err (content types only)
	Validated     int // Valid != nil
	Invalid       int // Valid != nil && *Valid == false
	NotValidated  int // Valid == nil
	SampleReadURI string
}

func TestCatalogContract(t *testing.T) {

	// Resolve sidecar binary paths up front; MakeTempUniverse-style skip
	// if any is missing (mirrors hubharness behaviour).
	var sidecars []hubharness.SidecarBin
	for _, name := range sidecarsToInclude {
		bin := binaryFor(t, name)
		if bin == "" {
			hubharness.MissingBinary(t, "catalog contract: sidecar binary %s not built", name)
			return
		}
		sidecars = append(sidecars, hubharness.SidecarBin{Name: name, Binary: bin})
	}

	// Build an isolated workspace whose .katyoti/config/Univers.yaml the
	// hub will resolve as ITS universe. The hub derives HomeDir from
	// KTW_WORKSPACE (= <ws>/.katyoti) — NOT from KTW_DATA — so we set
	// KTW_WORKSPACE before starting the hub. hubharness.Start spawns the
	// hub with os.Environ() (+KTW_DATA), so the inherited KTW_WORKSPACE
	// reaches the hub. This deliberately steers the hub AWAY from the
	// repo's live .katyoti onto our throwaway universe.
	ws := makeWorkspace(t, sidecars)
	dataDir := hubharness.TempDir(t) // hub lockfile + state, independent of HomeDir; its sidecars may hold their logs a moment after the hub stops

	// Set KTW_WORKSPACE for both the spawned hub (inherited via Start) and
	// this process's own resolution. Restore on cleanup.
	prevWS, hadWS := os.LookupEnv("KTW_WORKSPACE")
	if err := os.Setenv("KTW_WORKSPACE", ws); err != nil {
		t.Fatalf("set KTW_WORKSPACE: %v", err)
	}
	t.Cleanup(func() {
		if hadWS {
			_ = os.Setenv("KTW_WORKSPACE", prevWS)
		} else {
			_ = os.Unsetenv("KTW_WORKSPACE")
		}
	})

	// Start the hub. hubharness.Start skips if bin/ktw-hub isn't built.
	hubharness.Start(t, dataDir)

	// Connect over the bus (ModeReuse — the hub is already running).
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{
		DataDir: dataDir,
		Mode:    hubclient.ModeReuse,
	})
	if err != nil {
		t.Fatalf("catalog contract: connect to hub (dataDir=%s): %v", dataDir, err)
	}
	defer hc.Close()

	sender := messagebus.NewSender(hc.MessageBus(), tenantID, universeName)

	// ── Drain ListContributions (paginated, index form) ───────────────
	all := drainIndex(t, ctx, sender)
	if len(all) == 0 {
		t.Fatalf("catalog contract: ListContributions returned zero contributions — the hub catalog is empty (hydration of %v failed?)", sidecarsToInclude)
	}
	t.Logf("ListContributions drained %d total contributions across %d sidecars", len(all), len(sidecarsToInclude))

	// Group by Type.
	byType := map[string][]contribapi.Contribution{}
	for _, c := range all {
		byType[c.Type] = append(byType[c.Type], c)
	}

	// ── Per-type assertions + tally ───────────────────────────────────
	results := map[string]*perTypeResult{}
	var invalids []contribapi.Contribution

	for typ, entries := range byType {
		r := &perTypeResult{Type: typ, Count: len(entries)}

		// Metadata presence (Name) — every advertised contribution
		// should carry picker metadata.
		for _, c := range entries {
			meta := c.ResolvedMetadata()
			if meta.Name == "" {
				t.Errorf("contract: %s contribution %s has empty Metadata.Name", typ, c.ResolvedURI())
			}
			// Validation tally.
			switch {
			case c.Valid == nil:
				r.NotValidated++
			default:
				r.Validated++
				if !*c.Valid {
					r.Invalid++
					invalids = append(invalids, c)
				}
			}
		}

		// Read-raw spot check — CONTENT types only. Pick the first entry
		// per type; assert Read returns non-empty content + a shape that
		// is the RAW body, not the descriptor.
		if contentTypes[typ] {
			sample := entries[0]
			r.SampleReadURI = sample.ResolvedURI()
			content, mediaType, rerr := readContent(ctx, sender, sample.ResolvedURI())
			if rerr != nil || len(content) == 0 {
				r.ReadFail++
				t.Errorf("contract: Read(%s) [type=%s] returned empty/err (content type MUST read raw body): content=%dB mediaType=%q err=%v",
					sample.ResolvedURI(), typ, len(content), mediaType, rerr)
			} else {
				r.ReadOK++
				assertRawBodyShape(t, typ, sample.ResolvedURI(), content, mediaType)
			}
		}

		// Substrate types — assert the type DEFINITION is retrievable
		// through the index (ListContributions) and the data lane
		// (GetBody). Do NOT assert read-raw. We list this one type and
		// require resident data OR a data hash GetBody resolves.
		if substrateTypes[typ] {
			assertSubstrateDefinitionRetrievable(t, ctx, sender, typ, entries[0])
		}

		results[typ] = r
	}

	// ── Report: per-type table ────────────────────────────────────────
	reportTable(t, results)

	// ── Validation summary + invalid report ───────────────────────────
	reportValidation(t, all, invalids)

	// Expectation: after the data-type optional-field fixes, the
	// substrate + persona + data-type kinds that DECLARE a schema should
	// validate. We FAIL on any invalid contribution among those kinds —
	// that's a real contract violation, not a test artifact. Kinds with
	// no declared Schema legitimately stay NotValidated (Skipped).
	for _, c := range invalids {
		t.Errorf("CONTRACT VIOLATION: %s [type=%s] is INVALID. errors=%v",
			c.ResolvedURI(), c.Type, c.ValidationErrors)
	}

	// Sanity: the substrate types we expect ktw-agents to bundle should
	// actually appear. If they don't, the catalog is missing the bundled
	// type packs — a real regression worth surfacing (not silently
	// passing).
	for _, want := range []string{"object-type", "edge-type"} {
		if _, ok := byType[want]; !ok {
			t.Errorf("contract: expected substrate type %q to be present in the aggregate catalog (ktw-agents bundles the CWM type packs); it is ABSENT", want)
		}
	}

	// ── Consume parity ────────────────────────────────────────────────
	// The aggregate catalog HAVING the types (asserted above) is only half
	// the contract — the live CWMService must actually CONSUME them into its
	// type registry. When the hub is unreachable the registry silently
	// falls back to ktw-agents' in-binary FLOOR (only ktw-agents' own
	// types), so every other sidecar's item/node types vanish — looking
	// like a registration bug. This asserts the registry includes every
	// catalog-advertised item/node type, guarding that consume seam.
	assertCWMConsumeParity(t, ctx, sender, byType)
}

// assertCWMConsumeParity drives the REAL consume seam
// (cwmlocal.LoadCWMTypesFromCatalog) over the REAL aggregate-catalog bodies
// and asserts every advertised node/item/edge type survives into the type
// set. The catalog HAVING the types (asserted above) is only half the
// contract — the consume must also DECODE each body into its registry type.
// A regression that is invisible to the catalog-side checks — a body that
// fails to unmarshal (the json.RawMessage-encoding family of bugs), or a
// type silently dropped — surfaces here as a missing name. Catalog-only
// types like ktw-methods' `deliverable` / `use-case` and ktw-compose's
// `triage-candidate` MUST all come through.
func assertCWMConsumeParity(t *testing.T, ctx context.Context, sender *messagebus.Sender, byType map[string][]contribapi.Contribution) {
	t.Helper()

	// Materialise the substrate contributions WITH bodies — the index is in
	// index form, so a data layer past the inline band is read through the
	// data lane (GetBody) — so the consume helper decodes the same bytes the
	// live CWMService would.
	var contribs []contribapi.Contribution
	for _, typ := range []string{"object-type", "edge-type"} {
		raw, err := sender.Call(ctx, "ktw-hub:/CatalogService/ListContributions", mbapi.CatalogListContributionsInput{
			Filter: contribapi.ListFilter{Types: []string{typ}},
		})
		if err != nil {
			t.Errorf("consume parity: list %s: %v", typ, err)
			return
		}
		var resp mbapi.CatalogListContributionsOutput
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Errorf("consume parity: decode %s listing: %v", typ, err)
			return
		}
		for _, c := range resp.Contributions {
			if len(c.Data()) == 0 && c.DataHash() != "" {
				braw, gerr := sender.Call(ctx, "ktw-hub:/CatalogService/GetBody",
					mbapi.GetBodyInput{Hash: c.DataHash(), URI: c.ResolvedURI()})
				if gerr != nil {
					t.Errorf("consume parity: GetBody(%s): %v", c.ResolvedURI(), gerr)
					continue
				}
				var gout mbapi.GetBodyOutput
				if err := json.Unmarshal(braw, &gout); err != nil {
					t.Errorf("consume parity: GetBody decode (%s): %v", c.ResolvedURI(), err)
					continue
				}
				c.SetData(gout.Content)
			}
			contribs = append(contribs, c)
		}
	}

	// Run the real consume helper over a catalog of the real bodies.
	cat := manifestlocal.NewMemoryCatalog(
		[]*mbapi.Manifest{{Provider: "aggregate", Contributions: contribs}},
		[]string{"aggregate"},
	)
	set := cwmlocal.LoadCWMTypesFromCatalog(ctx, cat)

	got := map[string]map[string]bool{
		"object-type": {},
		"edge-type":   {},
	}
	for _, ot := range set.Objects {
		got["object-type"][ot.Name] = true
	}
	for _, et := range set.Edges {
		got["edge-type"][string(et.Name)] = true
	}

	for _, catalogType := range []string{"object-type", "edge-type"} {
		var missing []string
		for _, c := range byType[catalogType] {
			name := c.ID
			if name == "" {
				name = lastSeg(c.ResolvedURI())
			}
			if !got[catalogType][name] {
				missing = append(missing, c.ResolvedURI())
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Errorf("CONSUME PARITY VIOLATION: %d catalog-advertised %s(s) did NOT survive LoadCWMTypesFromCatalog (body failed to decode, or consume dropped it): %v",
				len(missing), catalogType, missing)
		} else {
			t.Logf("consume parity OK: all %d catalog %s(s) decoded into the type set", len(byType[catalogType]), catalogType)
		}
	}
}

// lastSeg returns the final path segment of a URI (after the last '/').
func lastSeg(uri string) string {
	if i := strings.LastIndex(uri, "/"); i >= 0 {
		return uri[i+1:]
	}
	return uri
}

// drainIndex pages through CatalogService.ListContributions (Limit>0,
// index form) following NextCursor until exhausted.
func drainIndex(t *testing.T, ctx context.Context, sender *messagebus.Sender) []contribapi.Contribution {
	t.Helper()
	var out []contribapi.Contribution
	cursor := ""
	const pageLimit = 200
	for page := 0; page < 1000; page++ {
		in := mbapi.CatalogListContributionsInput{
			Limit:  pageLimit,
			Cursor: cursor,
		}
		raw, err := sender.Call(ctx, "ktw-hub:/CatalogService/ListContributions", in)
		if err != nil {
			t.Fatalf("ListContributions (cursor=%q): %v", cursor, err)
		}
		var resp mbapi.CatalogListContributionsOutput
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatalf("ListContributions decode (cursor=%q): %v", cursor, err)
		}
		out = append(out, resp.Contributions...)
		if !resp.Pagination.HasMore || resp.Pagination.NextCursor == "" {
			break
		}
		cursor = resp.Pagination.NextCursor
	}
	return out
}

// readContent calls CatalogService.Read for one URI.
func readContent(ctx context.Context, sender *messagebus.Sender, uri string) ([]byte, string, error) {
	raw, err := sender.Call(ctx, "ktw-hub:/CatalogService/Read", mbapi.CatalogReadInput{URI: uri})
	if err != nil {
		return nil, "", err
	}
	var out mbapi.CatalogReadOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, "", fmt.Errorf("decode CatalogReadOutput: %w", err)
	}
	return out.Content, out.MediaType, nil
}

// assertRawBodyShape spot-checks that Read returned the RAW readable body
// for a content type, not the descriptor JSON. The key anti-pattern: a
// skill Read returning a `{"name":...}` SkillBody descriptor instead of
// the SKILL.md markdown means Read leaked the index descriptor.
func assertRawBodyShape(t *testing.T, typ, uri string, content []byte, mediaType string) {
	t.Helper()
	trimmed := strings.TrimSpace(string(content))

	switch typ {
	case "skill", "persona":
		// SKILL.md / persona markdown starts with YAML frontmatter `---`.
		// A descriptor leak would be a JSON object starting with `{`.
		if strings.HasPrefix(trimmed, "{") && looksLikeDescriptor(trimmed) {
			t.Errorf("contract: Read(%s) [type=%s] returned a JSON DESCRIPTOR, not the raw body (leaked descriptor): %.120s", uri, typ, trimmed)
		}
	case "data-type":
		// DSL is YAML — should contain `$schema:` or a YAML key, not be a
		// bare descriptor object. Tolerate JSON-AST form too, but flag a
		// descriptor-shaped object.
		if strings.HasPrefix(trimmed, "{") && looksLikeDescriptor(trimmed) {
			t.Errorf("contract: Read(%s) [type=data-type] returned a descriptor, not the DSL body: %.120s", uri, trimmed)
		}
	case "task":
		// task TASK.md is markdown with frontmatter; some providers emit
		// JSON. Either way it must be non-empty (already checked). Just
		// confirm it isn't an obviously-empty stub.
		if trimmed == "" {
			t.Errorf("contract: Read(%s) [type=task] returned blank content", uri)
		}
	}
	// Non-fatal observability: log what we got so a human can eyeball.
	t.Logf("read OK  %-14s %-48s media=%-22s %dB head=%q", typ, uri, mediaType, len(content), head(trimmed, 60))
}

// looksLikeDescriptor reports whether a JSON object looks like a
// Contribution/SkillBody descriptor (has top-level "name"/"provider"/
// "type" keys), as opposed to legitimate body JSON.
func looksLikeDescriptor(s string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return false
	}
	_, hasProvider := m["provider"]
	_, hasType := m["type"]
	_, hasID := m["id"]
	// provider+type+id together is the Contribution envelope signature.
	return hasProvider && hasType && hasID
}

// assertSubstrateDefinitionRetrievable confirms a substrate type's
// DEFINITION is retrievable through the index (ListContributions) and the
// data lane (GetBody) — its data layer IS the NodeType/ItemTypeRegistration
// JSON. We do NOT assert read-raw for substrate types.
func assertSubstrateDefinitionRetrievable(t *testing.T, ctx context.Context, sender *messagebus.Sender, typ string, sample contribapi.Contribution) {
	t.Helper()
	in := mbapi.CatalogListContributionsInput{
		Filter: contribapi.ListFilter{Types: []string{typ}},
		// Unpaginated: a single type's entries fit in one NATS message.
	}
	raw, err := sender.Call(ctx, "ktw-hub:/CatalogService/ListContributions", in)
	if err != nil {
		t.Errorf("contract: substrate %s ListContributions: %v", typ, err)
		return
	}
	var resp mbapi.CatalogListContributionsOutput
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Errorf("contract: substrate %s decode: %v", typ, err)
		return
	}
	if len(resp.Contributions) == 0 {
		t.Errorf("contract: substrate %s listing returned nothing", typ)
		return
	}
	// At least one entry must carry the type DEFINITION — either resident
	// data OR (index plane) a hash that GetBody can resolve.
	def := resp.Contributions[0]
	if len(def.Data()) == 0 && def.DataHash() == "" {
		t.Errorf("contract: substrate %s entry %s has neither resident data nor a data hash — type definition not retrievable", typ, def.ResolvedURI())
		return
	}
	// If the data is not resident, confirm GetBody resolves it by hash.
	bodyLen := len(def.Data())
	if bodyLen == 0 && def.DataHash() != "" {
		braw, gerr := sender.Call(ctx, "ktw-hub:/CatalogService/GetBody",
			mbapi.GetBodyInput{Hash: def.DataHash(), URI: def.ResolvedURI()})
		if gerr != nil {
			t.Errorf("contract: substrate %s GetBody(%s): %v", typ, def.ResolvedURI(), gerr)
			return
		}
		var gout mbapi.GetBodyOutput
		if err := json.Unmarshal(braw, &gout); err != nil {
			t.Errorf("contract: substrate %s GetBody decode: %v", typ, err)
			return
		}
		bodyLen = len(gout.Content)
		if bodyLen == 0 {
			t.Errorf("contract: substrate %s GetBody(%s) returned empty body", typ, def.ResolvedURI())
			return
		}
	}
	t.Logf("substrate %-10s definition retrievable: %s (%dB definition)", typ, def.ResolvedURI(), bodyLen)
}

// reportTable logs the per-type results table sorted by type name.
func reportTable(t *testing.T, results map[string]*perTypeResult) {
	t.Helper()
	types := make([]string, 0, len(results))
	for k := range results {
		types = append(types, k)
	}
	sort.Strings(types)

	var b strings.Builder
	b.WriteString("\n=== Per-type contract results ===\n")
	b.WriteString(fmt.Sprintf("%-16s %6s %8s %8s %10s %8s %12s  %s\n",
		"TYPE", "COUNT", "READ_OK", "READ_FAIL", "VALIDATED", "INVALID", "NOT_VALID", "CATEGORY"))
	for _, typ := range types {
		r := results[typ]
		cat := "other"
		if contentTypes[typ] {
			cat = "content"
		} else if substrateTypes[typ] {
			cat = "substrate"
		}
		b.WriteString(fmt.Sprintf("%-16s %6d %8d %8d %10d %8d %12d  %s\n",
			r.Type, r.Count, r.ReadOK, r.ReadFail, r.Validated, r.Invalid, r.NotValidated, cat))
	}
	t.Log(b.String())
}

// reportValidation logs an overall validation tally + every invalid URI.
func reportValidation(t *testing.T, all []contribapi.Contribution, invalids []contribapi.Contribution) {
	t.Helper()
	var validated, valid, invalid, notValidated int
	for _, c := range all {
		if c.Valid == nil {
			notValidated++
			continue
		}
		validated++
		if *c.Valid {
			valid++
		} else {
			invalid++
		}
	}
	t.Logf("=== Validation summary === total=%d validated=%d (valid=%d invalid=%d) not-validated(skipped)=%d",
		len(all), validated, valid, invalid, notValidated)
	if validated == 0 {
		t.Logf("NOTE: NO contribution carries a populated Valid field. Either the hub's Phase-2 Reindex/SetSchema didn't run, or no KIND declares a schema. This is reported, not silently passed.")
	}
	for _, c := range invalids {
		t.Logf("INVALID  %-48s errors=%v", c.ResolvedURI(), c.ValidationErrors)
	}
}

// ── helpers ──────────────────────────────────────────────────────────

// makeWorkspace builds a throwaway workspace dir and writes
// <ws>/.katyoti/config/Univers.yaml with a LOWERCASE universe name (so
// the in-Go subject sender doesn't panic) + the given sidecars as stdio
// entries. Returns the workspace ROOT (the value for KTW_WORKSPACE); the
// hub derives HomeDir = <ws>/.katyoti from it. Layout mirrors the real
// per-workspace layout (HomeDir = <KTW_WORKSPACE>/.katyoti).
func makeWorkspace(t *testing.T, sidecars []hubharness.SidecarBin) string {
	t.Helper()
	ws := hubharness.TempDir(t)
	cfgDir := filepath.Join(ws, ".katyoti", "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	var block strings.Builder
	for _, sc := range sidecars {
		block.WriteString(fmt.Sprintf("\n  - name: %s\n    transport: stdio\n    binary: %s\n", sc.Name, sc.Binary))
	}
	manifest := fmt.Sprintf(`version: 1
name: %s
display_name: Catalog Contract
icon: T
color: gray
sidecars:%s
`, universeName, block.String())
	if err := os.WriteFile(filepath.Join(cfgDir, "Univers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write Univers.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "Settings.yaml"), []byte("# catalog contract test universe\n"), 0o644); err != nil {
		t.Fatalf("write Settings.yaml: %v", err)
	}
	return ws
}

// repoRoot walks up from CWD to find go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("cwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod above %s", wd)
	return ""
}

// binaryFor answers the test's own copy of bin/<name>[.exe]
// (hubharness.Binary), "" when it is not built: bin/ stays replaceable while
// the suite runs.
func binaryFor(t *testing.T, name string) string {
	t.Helper()
	return hubharness.Binary(t, name)
}

// head returns the first n runes of s, single-lined.
func head(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= n {
		return s
	}
	return s[:n]
}
