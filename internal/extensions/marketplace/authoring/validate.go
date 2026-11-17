// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"context"
	"fmt"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// validate.go — checking a files-only pack before it ships.
//
// A compiled binary validates what it publishes in its own test suite, where
// the parser is a test-time dependency and never ships. A files-only pack has
// no test suite — someone scaffolds a folder, writes markdown and YAML, and
// packages it. That is the ONE path with no compiler in the loop, and therefore
// the one that most needs the check.
//
// So the check happens here, at packaging: the same framework function the
// aggregator runs at ingest, called at the moment that matters for content.

// Finding is one problem with a pack's content.
type Finding struct {
	URI      string
	Severity string // contribapi.ValidateWarn or ValidateError
	Messages []string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s [%s] %s", f.URI, f.Severity, strings.Join(f.Messages, "; "))
}

// ValidatePack checks every contribution in `cs` against the schema its kind
// declares, and returns what is wrong.
//
// Takes the contributions rather than probing for them, so a caller that has
// already asked the folder what it publishes — to register its schemas, say —
// asks once.
//
// `kinds` is the kind registry — the pack's own kinds and the host's system
// meta-model together. A schema and a severity are a kind's statements, so
// validating an entry means asking its kind; `cfg` only overrides.
//
// `validate` is the body check — nil when no validator is available. Nil is NOT
// treated as "everything passed": the framework records "no schema service
// available" as a finding under the configured severity, so a packaging run
// without a validator reports that it could not check rather than implying it
// did.
//
// Reports rather than refuses. Whether a finding stops a build is the caller's
// decision, taken with the severity the kind declares — see RefuseOn.
func ValidatePack(
	ctx context.Context,
	cs []contribapi.Contribution,
	cfg contribapi.ContributionsConfig,
	kinds contribapi.Kinds,
	validate manifestlocal.BodyValidator,
) []Finding {
	var out []Finding
	for _, c := range cs {
		outcome := manifestlocal.ValidateContribution(ctx, kinds.SeverityFor(c.Type, cfg), c, kinds.SchemaFor(c.Type), validate)
		if outcome.Skipped || len(outcome.Errors) == 0 {
			continue
		}
		severity := contribapi.ValidateWarn
		if outcome.Valid != nil && !*outcome.Valid {
			severity = contribapi.ValidateError
		}
		out = append(out, Finding{
			URI: c.ResolvedURI(), Severity: severity, Messages: outcome.Errors,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URI < out[j].URI })
	return out
}

// RefuseOn returns an error naming what failed, when any finding is severe
// enough to stop a build.
//
// Only `error`-severity findings refuse. A warning is a warning precisely so a
// pack whose kind declares `validate: warn` still ships — the severity is the
// kind author's decision about their own content, and packaging honours it
// rather than overriding it.
func RefuseOn(findings []Finding) error {
	var blocking []string
	for _, f := range findings {
		if f.Severity == contribapi.ValidateError {
			blocking = append(blocking, f.String())
		}
	}
	if len(blocking) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to package: %d contribution(s) did not pass validation:\n  %s",
		len(blocking), strings.Join(blocking, "\n  "))
}

// PackagingConfig resolves the severity packaging validates under, from what
// the kinds involved declare.
//
// `declared` maps a kind's name to the `validate:` it declares — the empty
// string for a kind that declares nothing, or that the caller could not find.
//
// **Packaging escalates.** A kind's own `validate:` answers a question about a
// LIVE catalog: how loud should an aggregator be about content that is already
// in the world. There, `warn` is right — a universe with one malformed skill
// should keep running, visibly flagged, rather than lose the other two hundred.
//
// Packaging asks a different question. Nothing has shipped, the author is
// standing right there, and the fix costs a minute. So every kind that is
// validated at all is validated strictly here, and a mismatch refuses.
//
// The one thing packaging does NOT override is `validate: off`. That is not a
// judgement about loudness; it is a kind saying its bodies are not describable
// by a schema. Escalating it would mean inventing a check the kind's author
// said does not exist.
func PackagingConfig(kinds contribapi.Kinds) contribapi.ContributionsConfig {
	cfg := contribapi.ContributionsConfig{
		Validate: contribapi.ValidateError,
		PerType:  map[string]string{},
	}
	for _, kind := range kinds.Names() {
		decl, _ := kinds.Get(kind)
		if strings.EqualFold(strings.TrimSpace(decl.Validate), contribapi.ValidateOff) {
			cfg.PerType[kind] = contribapi.ValidateOff
			continue
		}
		cfg.PerType[kind] = contribapi.ValidateError
	}
	return cfg
}

// FindingsForDrops turns files a host could not publish into findings.
//
// Always error severity, and not configurable. Every other finding is about
// content that IS in the pack and might be wrong; this one is about content
// that is NOT in the pack at all. There is no degraded-but-usable version of
// absent — a consumer reading the pack finds nothing there and has no way to
// learn that something was meant to be.
//
// It is also the one failure the schema check structurally cannot reach: a
// dropped file produces no contribution, so there is nothing to check.
func FindingsForDrops(drops []contribapi.Drop) []Finding {
	out := make([]Finding, 0, len(drops))
	for _, d := range drops {
		kind := d.Kind
		if kind == "" {
			kind = "content"
		}
		out = append(out, Finding{
			URI:      d.Path,
			Severity: contribapi.ValidateError,
			Messages: []string{fmt.Sprintf("published nothing — this file was not read as a %s: %s", kind, d.Reason)},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URI < out[j].URI })
	return out
}
