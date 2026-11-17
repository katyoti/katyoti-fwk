// SPDX-License-Identifier: Apache-2.0

package catalog

// Exhaustive per-resource contract: for EVERY contribution the hub serves,
// assert that get (GetBody) + read (Read) work, and that the body validates
// against its declared resource-type schema (independently, via the hub's
// SchemaService.ValidateAgainst — NOT just trusting the hub's ingest-time
// Valid flag). This is the "validate every resource against its type schema +
// prove get/read" suite requested on top of the per-type contract test.
//
// Run via:  ktw-test test ./internal/test/catalog/
// Skips cleanly (hubharness) when bin/ktw-hub or a sidecar binary is missing.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	schemaextapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// resourceFailure is one (resource, phase) contract violation.
type resourceFailure struct {
	URI    string
	Type   string
	Phase  string // "get" | "read" | "validate"
	Detail string
}

func TestResourceValidationContract(t *testing.T) {

	var sidecars []hubharness.SidecarBin
	for _, name := range sidecarsToInclude {
		bin := binaryFor(t, name)
		if bin == "" {
			hubharness.MissingBinary(t, "resource validation: sidecar binary %s not built", name)
			return
		}
		sidecars = append(sidecars, hubharness.SidecarBin{Name: name, Binary: bin})
	}

	ws := makeWorkspace(t, sidecars)
	dataDir := t.TempDir()

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

	hubharness.Start(t, dataDir)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: dataDir, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("resource validation: connect to hub: %v", err)
	}
	defer hc.Close()
	sender := messagebus.NewSender(hc.MessageBus(), tenantID, universeName)

	// Drain EVERY contribution in index form; each body is then read
	// through the data lane (GetBody), which is also the "get" contract.
	all := drainIndex(t, ctx, sender)
	if len(all) == 0 {
		t.Fatalf("resource validation: catalog empty (hydration of %v failed?)", sidecarsToInclude)
	}
	t.Logf("validating %d resources across %d sidecars", len(all), len(sidecarsToInclude))

	// Warm-up: force-spawn each owning sidecar ONCE, sequentially, before the
	// concurrent loop. Read proxies to the owning sidecar's ResourceService;
	// without warm-up the concurrent first-touch storm cold-spawns every sidecar
	// at once → "transport: closed" / timeouts. One sequential read per provider
	// connects + warms the connmgr so the parallel phase is fast and reliable.
	warmed := map[string]bool{}
	for _, c := range all {
		if !contentTypes[c.Type] || warmed[c.Provider] {
			continue
		}
		warmed[c.Provider] = true
		_, _, _ = readContent(ctx, sender, c.ResolvedURI()) // best-effort spawn
	}
	t.Logf("warmed %d sidecars", len(warmed))

	var (
		mu       sync.Mutex
		fails    []resourceFailure
		nGet     int
		nRead    int
		nValid   int
		nSkipVal int
	)
	record := func(f resourceFailure) {
		mu.Lock()
		fails = append(fails, f)
		mu.Unlock()
	}

	// A schema is the KIND's statement, so resolution is a lookup built once
	// from the kind declarations the universe carries.
	kinds := map[string]string{}
	for _, c := range all {
		if c.Type != contribapi.ResourceTypeContributionType {
			continue
		}
		data := c.Data()
		if len(data) == 0 && c.DataHash() != "" {
			body, gerr := getBody(ctx, sender, c.DataHash(), c.ResolvedURI())
			if gerr != nil {
				t.Fatalf("resource validation: GetBody(%s) for the kind declaration: %v", c.ResolvedURI(), gerr)
			}
			data = body
		}
		if len(data) == 0 {
			continue
		}
		var decl contribapi.KindDeclaration
		if err := json.Unmarshal(data, &decl); err != nil || decl.Schema == "" {
			continue
		}
		kind := decl.Name
		if kind == "" {
			kind = c.ID
		}
		kinds[kind] = decl.Schema
	}

	sem := make(chan struct{}, 8) // bound concurrent bus RPCs (sidecars pre-warmed above)
	var wg sync.WaitGroup
	for _, c := range all {
		c := c
		uri := c.ResolvedURI()
		schema := mbapi.SchemaFor(kinds, c.Type)

		// Metadata (the "get" contract's L0): every advertised resource must
		// carry a non-empty Name.
		if c.ResolvedMetadata().Name == "" {
			record(resourceFailure{uri, c.Type, "get", "empty Metadata.Name"})
		}

		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			// GET — the per-URI body fetch (the data lane). Only assert for
			// resources that actually HAVE a body (a data hash or resident
			// data); bodyless kinds (service / sidecar-info) legitimately
			// return empty. The body read here is the one validated below.
			var data []byte
			if c.DataHash() != "" || len(c.Data()) > 0 {
				body, gerr := getBody(ctx, sender, c.DataHash(), uri)
				if gerr != nil {
					record(resourceFailure{uri, c.Type, "get", "GetBody error: " + gerr.Error()})
				} else if len(body) == 0 {
					record(resourceFailure{uri, c.Type, "get", "GetBody returned empty for a body-bearing resource"})
				} else {
					data = body
					mu.Lock()
					nGet++
					mu.Unlock()
				}
			}

			// READ — the raw readable content. Content kinds MUST return a
			// non-empty raw body.
			if contentTypes[c.Type] {
				content, rerr := busRetry(func() ([]byte, error) {
					b, _, e := readContent(ctx, sender, uri)
					return b, e
				})
				if rerr != nil {
					record(resourceFailure{uri, c.Type, "read", "Read error: " + rerr.Error()})
				} else if len(content) == 0 {
					record(resourceFailure{uri, c.Type, "read", "Read returned empty raw body"})
				} else {
					mu.Lock()
					nRead++
					mu.Unlock()
				}
			}

			// VALIDATE — independently validate the data layer against the
			// resource's declared type schema via the hub's SchemaService.
			if schema == "" || len(data) == 0 {
				mu.Lock()
				nSkipVal++
				mu.Unlock()
				return
			}
			out, verr := validateAgainstSchema(ctx, sender, schema, data)
			if verr != nil {
				record(resourceFailure{uri, c.Type, "validate", fmt.Sprintf("ValidateAgainst(%s) error: %v", schema, verr)})
				return
			}
			if !out.Valid {
				record(resourceFailure{uri, c.Type, "validate", fmt.Sprintf("body INVALID vs %s: %s", schema, diagSummary(out.Diagnostics))})
				return
			}
			// Cross-check: independent validation must agree with the hub's
			// ingest-time Valid flag (when the hub validated this kind).
			if c.Valid != nil && !*c.Valid {
				record(resourceFailure{uri, c.Type, "validate", fmt.Sprintf("hub ingest flag says INVALID but ValidateAgainst(%s) says valid (disagreement)", schema)})
				return
			}
			mu.Lock()
			nValid++
			mu.Unlock()
		}()
	}
	wg.Wait()

	t.Logf("OK: get=%d  read=%d  schema-validated=%d  (no-schema skipped=%d)", nGet, nRead, nValid, nSkipVal)

	if len(fails) > 0 {
		// Group by phase for a readable report.
		byPhase := map[string]int{}
		for _, f := range fails {
			byPhase[f.Phase]++
		}
		t.Errorf("resource validation: %d contract violation(s) — get=%d read=%d validate=%d",
			len(fails), byPhase["get"], byPhase["read"], byPhase["validate"])
		sort.Slice(fails, func(i, j int) bool {
			if fails[i].Phase != fails[j].Phase {
				return fails[i].Phase < fails[j].Phase
			}
			return fails[i].URI < fails[j].URI
		})
		const cap = 50
		for i, f := range fails {
			if i >= cap {
				t.Logf("  ... +%d more", len(fails)-cap)
				break
			}
			t.Logf("  [%s] %-52s %s", f.Phase, f.URI, f.Detail)
		}
	}
}

// busRetry retries a bus call on transient transport errors — the proxy paths
// (GetBody for layer/uncached entries, Read) lazily spawn the owning sidecar via
// the connmgr, so the first concurrent touch can time out or hit a recycled
// connection ("transport: closed"). A real contract violation survives the
// retries; a cold-spawn hiccup does not.
func busRetry[T any](fn func() (T, error)) (T, error) {
	var last error
	var zero T
	for attempt := 0; attempt < 2; attempt++ {
		v, err := fn()
		if err == nil {
			return v, nil
		}
		last = err
		if !isTransientBusErr(err) {
			return zero, err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return zero, last
}

func isTransientBusErr(err error) bool {
	s := err.Error()
	for _, m := range []string{"transport: closed", "request timeout", "no responders", "connection closed"} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// getBody calls CatalogService.GetBody for one contribution (by content hash).
func getBody(ctx context.Context, sender *messagebus.Sender, hash, uri string) ([]byte, error) {
	return busRetry(func() ([]byte, error) {
		raw, err := sender.Call(ctx, "ktw-hub:/CatalogService/GetBody", mbapi.GetBodyInput{Hash: hash, URI: uri})
		if err != nil {
			return nil, err
		}
		var out mbapi.GetBodyOutput
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("decode GetBodyOutput: %w", err)
		}
		return out.Content, nil
	})
}

// validateAgainstSchema calls the hub's SchemaService.ValidateAgainst.
func validateAgainstSchema(ctx context.Context, sender *messagebus.Sender, schemaURI string, body []byte) (schemaapi.ValidateOutput, error) {
	raw, err := sender.Call(ctx, "ktw-hub:/schema/ValidateAgainst", schemaextapi.ValidateAgainstInput{
		URI:  schemaURI,
		Body: json.RawMessage(body),
	})
	if err != nil {
		return schemaapi.ValidateOutput{}, err
	}
	var out schemaapi.ValidateOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return schemaapi.ValidateOutput{}, fmt.Errorf("decode ValidateOutput: %w", err)
	}
	return out, nil
}

func diagSummary(diags []schemaapi.Diagnostic) string {
	if len(diags) == 0 {
		return "(no diagnostics)"
	}
	out := ""
	for i, d := range diags {
		if i >= 3 {
			out += fmt.Sprintf(" (+%d more)", len(diags)-3)
			break
		}
		if i > 0 {
			out += "; "
		}
		out += fmt.Sprintf("%s@%s", d.Message, d.Path)
	}
	return out
}
