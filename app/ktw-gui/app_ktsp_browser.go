// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"sort"
	"strings"
	"sync"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// KTSPManifestSummary is the GUI-facing slice of a sidecar's manifest
// (its callable lane): provider identity, the slot instances it publishes, and
// the methods each instance exposes (resolved from the matching
// SlotType in the same manifest, so the UI doesn't have to
// cross-reference).
type KTSPManifestSummary struct {
	Provider    string            `json:"provider"`
	Version     string            `json:"version,omitempty"`
	Description string            `json:"description,omitempty"`
	Slots       []KTSPSlotSummary `json:"slots"`
}

// KTSPSlotSummary is one slot instance on a provider plus the methods
// the slot type contributes. The KTSP wire address is
// `<provider>:<slot.Name>/<method.Name>` for the browser's invoke
// pane.
type KTSPSlotSummary struct {
	Name        string              `json:"name"`
	Type        string              `json:"type"`
	Description string              `json:"description,omitempty"`
	Methods     []KTSPMethodSummary `json:"methods"`
}

// KTSPMethodSummary is one method on a slot. InputSchema /
// OutputSchema are the JSON Schemas auto-generated from the Go input
// and output types at AutoKTSPContainer time — passed through as raw
// JSON so the frontend can render them with the existing
// JSON-Schema-aware widgets without a second round trip.
type KTSPMethodSummary struct {
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema,omitempty"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
}

// ListKTSPManifestsResult is the Wails return for the KTSP browser's
// initial tree fetch. Error is surfaced inline so the frontend can
// show a banner instead of swallowing. Unavailable names each member whose
// manifest could not be read, with the reason: one broken member is shown
// beside the others, it does not hide them.
type ListKTSPManifestsResult struct {
	Manifests   []KTSPManifestSummary `json:"manifests"`
	Unavailable []string              `json:"unavailable,omitempty"`
	Error       string                `json:"error,omitempty"`
}

// ListKTSPManifests returns the manifest of every provider known to the
// universe's hub catalog (served from the hub's tier-1 manifest cache —
// pure in-memory enumeration on the hub side, no sidecar spawn).
//
// Implementation note: there is no bulk "every manifest" method on
// CatalogService, on purpose: every manifest's full payload (slot types +
// JSON Schemas + every Contribution) in one reply would blow past the bus's
// per-message size limit. So: membership first (`remote.Catalog.Providers`,
// over `CatalogService/ListNames`, tiny), then a parallel
// `CatalogService/GetManifest` per name, each manifest in its own reply
// (modules/contributions/contract-matrix.md rows 12 and 13).
//
// Wails-bound: window.go.main.App.ListKTSPManifests(universeName).
func (a *App) ListKTSPManifests(universeName string) ListKTSPManifestsResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return ListKTSPManifestsResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}

	cat, err := a.universeCatalog(universeName)
	if err != nil {
		return ListKTSPManifestsResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	// Membership, not just what has been probed: a member whose manifest is
	// not cached yet must still appear, or it looks identical to one that does
	// not exist.
	providers, err := cat.Providers(a.ctx)
	if err != nil {
		return ListKTSPManifestsResult{Error: fmt.Sprintf("list providers: %v", err)}
	}

	// Fan out GetManifest per provider — each response is bounded by one
	// sidecar's payload, which fits comfortably in a single bus message.
	//
	// `ListNames` enumerates the universe's members (the `sidecars:` block
	// of Univers.yaml), which is SIDECARS only — ktw-hub is the
	// orchestrator (publishes CatalogService / SchemaService /
	// ConnMgrService) but is not one of its own members and has no cache
	// folder. So the hub IS reachable on the bus yet
	// never appears in `ListNames`. The browser fetches it directly via
	// its own ResourceService instance to surface the hub's slots
	// alongside the sidecars'.
	type namedFetch struct {
		name string
		// path/input are set ONLY for the hub's own manifest, which the
		// aggregator cannot serve (see below). Every other member goes
		// through the catalog reader.
		path  string
		input any
	}
	fetches := make([]namedFetch, 0, len(providers)+1)
	for _, n := range providers {
		fetches = append(fetches, namedFetch{name: n})
	}
	fetches = append(fetches, namedFetch{
		name:  "ktw-hub",
		path:  "ktw-hub:resources/get-manifest",
		input: mbapi.ContributionGetManifestInput{},
	})

	manifests := make([]*mbapi.Manifest, len(fetches))
	errs := make([]error, len(fetches))
	var wg sync.WaitGroup
	for i, f := range fetches {
		i, f := i, f
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Ordinary members: through the reader, which owns the
			// aggregator's address.
			if f.path == "" {
				m, err := cat.Manifest(a.ctx, f.name)
				if err != nil {
					errs[i] = fmt.Errorf("%s: %w", f.name, err)
					return
				}
				manifests[i] = m
				return
			}
			// The hub's OWN manifest, which the aggregator cannot serve: it
			// enumerates members from Univers.yaml, and the hub is not one
			// of its own members. Asked of the hub's ResourceService
			// directly — a slot method, not a catalog address.
			respRaw, err := sender.Call(a.ctx, f.path, f.input)
			if err != nil {
				errs[i] = fmt.Errorf("%s: %w", f.name, err)
				return
			}
			var resp mbapi.ContributionGetManifestOutput
			if err := json.Unmarshal(respRaw, &resp); err != nil {
				errs[i] = fmt.Errorf("%s decode: %w", f.name, err)
				return
			}
			manifests[i] = resp.Manifest
		}()
	}
	wg.Wait()

	// A member whose manifest cannot be read is reported beside the others,
	// never in their place: the user sees what the catalog holds AND which
	// member it is missing. The dedicated ktw-hub fetch stays best-effort —
	// if the hub doesn't publish ResourceService, the sidecars still render.
	var unavailable []string
	for i, e := range errs {
		if e == nil {
			continue
		}
		manifests[i] = nil
		if fetches[i].name == "ktw-hub" {
			continue
		}
		unavailable = append(unavailable, e.Error())
	}
	sort.Strings(unavailable)

	out := make([]KTSPManifestSummary, 0, len(manifests))
	for _, m := range manifests {
		if m == nil {
			continue
		}
		// Index slot-types by their type discriminator so each Slot
		// instance can pull in the method list from its declared type.
		typeMethods := make(map[string][]mbapi.SlotMethod, len(m.SlotTypes))
		for _, st := range m.SlotTypes {
			typeMethods[st.Type] = st.Methods
		}

		slots := make([]KTSPSlotSummary, 0, len(m.Slots))
		for _, s := range m.Slots {
			methods := typeMethods[s.Type]
			ms := make([]KTSPMethodSummary, 0, len(methods))
			for _, sm := range methods {
				ms = append(ms, KTSPMethodSummary{
					Name:         sm.Name,
					Description:  sm.Description,
					InputSchema:  sm.InputSchema,
					OutputSchema: sm.OutputSchema,
					Tags:         sm.Tags,
				})
			}
			slots = append(slots, KTSPSlotSummary{
				Name:        s.Name,
				Type:        s.Type,
				Description: s.Description,
				Methods:     ms,
			})
		}

		out = append(out, KTSPManifestSummary{
			Provider:    m.Provider,
			Version:     m.Version,
			Description: m.Description,
			Slots:       slots,
		})
	}
	// Stable order by provider so the tree doesn't shuffle between
	// fetches — fan-out order is non-deterministic.
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return ListKTSPManifestsResult{Manifests: out, Unavailable: unavailable}
}

// KTSPCallResult is the Wails return of a debug-tool KTSP call.
// Either Response or Error is set.
type KTSPCallResult struct {
	Response json.RawMessage `json:"response,omitempty"`
	Error    string          `json:"error,omitempty"`
	// ErrorMsg is the error by key, in the person's language (errors namespace).
	ErrorMsg *usermsg.Msg `json:"errorMsg,omitempty"`
}

// CallKTSPMethod sends an arbitrary KTSP call.
//
// This is the GENERAL way the GUI reaches a published service method — not a
// debug-only tool. Per management-plane-hub-authority invariant 5, a new service
// method must be callable from the frontend the day it is published, with no Go
// change here; adding a Wails binding per method would rebuild the whole API
// surface a second time in Go and drift from the ServiceDef.
// `slotPath` is the canonical wire form `<provider>:<slot>/<method>`
// (e.g. `ktw-hub:schema/get-data-type`). `inputJSON` is the request body
// serialised as JSON; we decode-then-pass so the substrate sees a
// typed `any` rather than a raw string.
//
// Wails-bound: window.go.main.App.CallKTSPMethod(universeName,
//
//	slotPath, inputJSON).
func (a *App) CallKTSPMethod(universeName, slotPath, inputJSON string) KTSPCallResult {
	if slotPath == "" {
		return KTSPCallResult{Error: "slot path is required (e.g. `ktw-hub:schema/get-data-type`)"}
	}
	sender, err := a.universeSender(universeName)
	if err != nil {
		return KTSPCallResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}

	var input any
	if inputJSON != "" {
		if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
			return KTSPCallResult{Error: fmt.Sprintf("Input is not valid JSON: %v", err)}
		}
	}

	// A few methods FETCH BYTES rather than read state, and the default
	// request budget is sized for the latter. Installing a 40 MB sidecar
	// over a slow link would otherwise time out mid-download and report a
	// failure for an operation that was progressing fine.
	//
	// A longer budget, not a job registry: these are still ONE call that
	// either completes or does not. An async id + poll would change the API
	// shape of every caller to solve a problem a timeout solves.
	//
	// Every other method gets the request timeout it declares (its
	// MethodDef.TimeoutS, read from the universe's catalog): a doctor that
	// starts each item's binary, or a plan on a cold machine, is not cut at
	// the transport's few seconds.
	ctx := a.ctx
	var manifestOf mbapi.ManifestOf
	if !isLongRunningKTSP(slotPath) {
		if c, err := a.universeCatalog(universeName); err == nil {
			manifestOf = c.ManifestSlots
		}
	}
	if budget := callBudget(ctx, manifestOf, slotPath); budget > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
	}
	respRaw, err := sender.Call(ctx, slotPath, input)
	if err != nil {
		return KTSPCallResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	}
	return KTSPCallResult{Response: respRaw}
}

// callBudget is how long a GUI call may wait: a byte-transferring method's
// long budget, else the method's declared timeout (0: the transport's
// default).
func callBudget(ctx context.Context, manifestOf mbapi.ManifestOf, slotPath string) time.Duration {
	if isLongRunningKTSP(slotPath) {
		return longKTSPCallBudget
	}
	return mbapi.MethodTimeout(ctx, manifestOf, slotPath)
}

// longKTSPCallBudget bounds a method that transfers bytes. Generous enough for a
// large sidecar on a poor connection, still finite: a call that hangs forever is
// worse than one that fails with a reason.
const longKTSPCallBudget = 10 * time.Minute

// isLongRunningKTSP matches by METHOD SUFFIX, so it holds for any provider or
// slot serving these verbs rather than hard-coding one wire path.
//
// Deliberately a short list. Every entry here is a method that downloads or
// unpacks; widening it to "anything that might be slow" would hide real hangs
// behind a ten-minute wait.
func isLongRunningKTSP(slotPath string) bool {
	for _, m := range []string{
		"/install", "/update", "/set-version", "/install-solution", "/import-vault", "/apply-seed",
		"/apply-config-set", "/configure-item", "/refresh-catalog", "/install-prereqs",
	} {
		if strings.HasSuffix(slotPath, m) {
			return true
		}
	}
	return false
}
