// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// BodyValidator is the minimal validation primitive both the catalog
// (ingest, hub-side) and the Builder (source, sidecar-side) call. It
// validates a JSON body against a data-type URI, returning a
// lint-friendly error (nil on success). Both
// mbapi.CatalogSchemaService.ValidateBody and an adapter over
// schema.SchemaService.ValidateAgainst satisfy this shape.
type BodyValidator func(ctx context.Context, dataTypeURI string, body json.RawMessage) error

// ValidationOutcome is the result of validating one contribution. It
// mirrors what the catalog stores on Contribution (Valid + errors) but
// is decoupled so both call sites (ingest + source) can act on it
// uniformly. Skipped reports whether the contribution was not validated
// at all (severity off) — distinct from "validated and passed".
type ValidationOutcome struct {
	Skipped bool
	Valid   *bool
	Errors  []string
}

// ValidateContribution is the SINGLE framework validation function. Unless
// `severity` is off, it validates the data layer against `schema`.
//
// Both the schema AND the severity are PASSED IN, resolved from the kind by the
// caller (contribapi.Kinds.SchemaFor / SeverityFor). The schema used to be a
// field on the contribution — a value with two writers; the severity used to
// come from a hard-coded per-kind map that duplicated the kind files and left
// the kind's own `validate:` unread. A kind declares both; an entry of that
// kind has them by being that kind. Central config may override the severity,
// which is the caller's business too (Kinds.SeverityFor takes it).
//
// Severity semantics:
//   - off   → Skipped=true; Valid=nil; no errors.
//   - warn  → diagnostics recorded in Errors; Valid stays nil (kept
//     valid; the caller logs).
//   - on    → diagnostics recorded in Errors; Valid=*false (kept-but-
//     flagged invalid).
//
// validate may be nil (no schema service available) — in that case a
// declared Schema cannot be checked, treated like the unresolvable-
// data-type warning path: recorded under severity, never a hard fail.
func ValidateContribution(
	ctx context.Context,
	severity string,
	c contribapi.Contribution,
	schema string,
	validate BodyValidator,
) ValidationOutcome {
	severity = contribapi.NormalizeSeverity(severity)
	if severity == contribapi.ValidateOff {
		return ValidationOutcome{Skipped: true}
	}

	// No structured body, nothing for a schema to check. A schema describes
	// the body a consumer DECODES, and three kinds of entry have none:
	//
	//   - bulk content — opaque bytes a consumer streams, with no body beside
	//     them; deriving one would mean holding content the band exists
	//     precisely because nobody can hold it.
	//   - an entry whose content lives only in the SOURCE layer. A data-type
	//     is the whole class: it IS its DSL, and since c24 it stops carrying a
	//     structured copy. Feeding the empty data layer to the validator made
	//     every one of them report `unexpected end of JSON input` — an entry
	//     called invalid for not having a thing its kind does not have.
	//   - an entry whose body is simply not resident on this side.
	//
	// A data-type's real check is that it PARSES, which registration performs
	// and `census.assertEveryDataTypeResolves` asserts. It is not this one.
	//
	// Skipped, not valid. "We did not check this" and "this passed" are
	// different answers, and a consumer applying a policy has to tell them
	// apart.
	if len(c.Data()) == 0 {
		return ValidationOutcome{Skipped: true}
	}

	// No schema: the KIND declares none, which is a finding about the kind
	// rather than about this entry — flagged under the same severity so it is
	// visible rather than silently treated as a pass.
	if schema == "" {
		return outcomeFor(severity, []string{
			fmt.Sprintf("kind %q declares no data-type", c.Type),
		})
	}

	if validate == nil {
		return outcomeFor(severity, []string{
			fmt.Sprintf("no schema service available to validate against %q", schema),
		})
	}

	if err := validate(ctx, schema, c.Data()); err != nil {
		return outcomeFor(severity, []string{err.Error()})
	}

	// Passed. Record an explicit *true so consumers can distinguish
	// "validated OK" from "not validated".
	pass := true
	return ValidationOutcome{Valid: &pass}
}

// outcomeFor builds a failing ValidationOutcome at the given (non-off)
// severity: warn keeps Valid nil; on sets Valid=*false.
func outcomeFor(severity string, errs []string) ValidationOutcome {
	out := ValidationOutcome{Errors: errs}
	if severity == contribapi.ValidateOn {
		invalid := false
		out.Valid = &invalid
	}
	return out
}

// VerdictFromRegistration turns a data-type's registration outcome into the
// verdict every consumer reads — the same shape ValidateContribution produces
// for a structured body, so a data-type is no longer the one kind that reads
// "unchecked" in every listing.
//
// Severity is the kind's (data-type declares `error`): a schema that does not
// parse is marked invalid and KEPT, with the parser's reason as its diagnostic;
// under `warn` the reason is recorded and Valid stays unset; under `off`
// nothing is recorded.
func VerdictFromRegistration(severity string, o mbapi.RegistrationOutcome) ValidationOutcome {
	severity = contribapi.NormalizeSeverity(severity)
	if severity == contribapi.ValidateOff {
		return ValidationOutcome{Skipped: true}
	}
	if o.Registered {
		pass := true
		return ValidationOutcome{Valid: &pass}
	}
	reason := o.Reason
	if reason == "" {
		reason = "schema did not register"
	}
	return outcomeFor(severity, []string{"data-type does not parse: " + reason})
}
