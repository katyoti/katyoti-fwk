// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	schemalocal "github.com/katyoti/katyoti-fwk/internal/extensions/schema/local"
)

// validate_pack.go — refusing to package content that does not match its own
// schemas.
//
// The validator is EMBEDDED here rather than borrowed over the bus. Measured
// cost: five packages, because ktw-compose already links most of the schema
// service's closure. Borrowing would have added an infrastructure dependency —
// a reachable aggregator — to an offline authoring flow, to save five packages.
// That is the wrong trade, and it would tax exactly the frictionless authoring
// the files-only path exists to provide.
//
// The rule that keeps the parser OUT of publish-only sidecars is unchanged and
// still matters: a binary that only ships content must not carry the machinery
// for checking it. ktw-compose is the opposite case — it is the thing
// exercising validation, and the cost of a capability belongs with whoever
// exercises it.

// checkPack probes a folder, teaches the local schema service every data-type
// involved, and returns what does not match.
//
// Two sets of contributions feed it, and both are needed: the PACK's own, and
// this binary's, read at `self` reach — the same query any consumer makes. The
// pack carries its own kinds and schemas; the kinds it uses without declaring
// (the system set, and the kinds this binary's packs declare under its own
// provider, skill among them) live in the binary. Asking for
// everything once rather than for data-types twice also yields the kind
// declarations, which decide how severe a mismatch is.
func checkPack(ctx context.Context, hostBinary, folder string) ([]authoring.Finding, error) {
	cs, drops, err := authoring.ProbeFolder(hostBinary, folder)
	if err != nil {
		return nil, err
	}
	// A file that published NOTHING is checked first, and separately: it is
	// the failure the schema check cannot reach, because a dropped file
	// produces no contribution to check.
	dropped := authoring.FindingsForDrops(drops)
	if len(cs) == 0 {
		return dropped, nil
	}

	// This binary's own contributions — the kinds and schemas a pack's
	// entries point at. A pack's skills validate against the skill data-type
	// this binary's skills pack ships, which the pack itself does not carry.
	var own []contribapi.Contribution
	if entries, qerr := mbapi.Query(ctx, mbapi.Q{Reach: mbapi.ReachSelf}); qerr == nil {
		own = entries
	}
	// The kinds involved, from BOTH sides: the pack declares its own, and the
	// kinds it uses without declaring live in this binary. A schema and a
	// severity are a kind's statements, so validating an entry means finding
	// its kind — and a pack's kind may be declared by either, or by both alike
	// (one kind: KindsFrom keeps the first, the binary's).
	kinds := contribapi.KindsFrom(append(own, cs...))
	cfg := authoring.PackagingConfig(kinds)

	svc, err := application.UseService[schemaapi.SchemaService](ctx)
	if err != nil || svc == nil {
		// No validator wired. ValidateContribution records that as a finding
		// rather than a pass, so the caller learns the check did not happen
		// instead of being told everything was fine.
		return append(dropped, authoring.ValidatePack(ctx, cs, cfg, kinds, nil)...), nil
	}

	_, ownFailed := schemalocal.RegisterDataTypes(ctx, svc, own, nil)
	_, packFailed := schemalocal.RegisterDataTypes(ctx, svc, cs, nil)

	findings := append(dropped, authoring.ValidatePack(ctx, cs, cfg, kinds, schemalocal.BodyValidatorFor(svc))...)
	return append(findings, unparseableSchemas(cs, append(ownFailed, packFailed...))...), nil
}

// unparseableSchemas turns data-types that would not register into findings.
//
// A data-type's BODY always validates: it is a `{format, source}` envelope, and
// a broken schema is still a valid string in `source`. Checking a schema
// against the schema-of-schemas therefore proves nothing about the schema —
// the only real check is whether the parser accepts it, which registration
// already answered.
//
// This refuses rather than warns, and refuses for the PACK's own schemas only.
// A schema that does not parse is not a degraded schema; it is an absence. Ship
// it and every URI pointing at it 404s in a consumer that cannot explain why —
// which is exactly the silent failure packaging exists to catch. A failure in
// the HOST's schemas is a bug in this binary, not in the pack, so it is logged
// (by RegisterDataTypes) and not charged to the author.
func unparseableSchemas(cs []contribapi.Contribution, failures []schemalocal.DataTypeFailure) []authoring.Finding {
	if len(failures) == 0 {
		return nil
	}
	own := map[string]bool{}
	for _, c := range cs {
		own[c.ResolvedURI()] = true
	}
	var out []authoring.Finding
	for _, f := range failures {
		if !own[f.URI] {
			continue
		}
		out = append(out, authoring.Finding{
			URI:      f.URI,
			Severity: contribapi.ValidateError,
			Messages: []string{"schema does not parse: " + f.Reason},
		})
	}
	return out
}

// declaredSeverities reads every `contribution-type` entry in `cs` and returns
// what each kind declares under `validate:`.
//
// A kind that declares nothing is still listed, with an empty severity — the
// difference between "this kind says warn" and "no declaration was found" is
// not one packaging acts on, but recording it keeps PackagingConfig's decision
// visible in one place instead of spread across two callers.
// refusePackIfInvalid runs the check and turns error-severity findings into a
// refusal, naming what failed.
//
// A probe failure is reported too: packaging that cannot see what it is about
// to ship has not validated it, and saying so is better than shipping quietly.
func refusePackIfInvalid(ctx context.Context, hostBinary, folder string) error {
	findings, err := checkPack(ctx, hostBinary, folder)
	if err != nil {
		return fmt.Errorf("validate %s: %w", folder, err)
	}
	return authoring.RefuseOn(findings)
}
