// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// register.go — teaching a SchemaService the data-types it will validate against.
//
// Two places need this: the aggregator at ingest, and the packaging tool, which
// checks a pack before it ships. They do the same thing for the same reason, so
// they do it through the same function — a second copy would be a second place
// for the envelope shape to drift, and the shape is the part that has already
// gone wrong once (see the empty-source warning below).

// DataTypeFailure is one data-type that did not make it into the registry, and
// why.
//
// Returned as well as logged, because the two callers need different things
// from the same event. A running aggregator logs and carries on — one bad
// schema must not cost a universe its other two hundred. Packaging has not
// shipped anything yet, so it can still refuse.
type DataTypeFailure struct {
	URI    string
	Reason string
}

// RegisterDataTypes registers every `data-type` contribution in `cs` with svc,
// and returns how many were registered alongside the ones that failed.
//
// `resolveSource` materialises a schema the index does not carry. A data-type's
// DSL is its `source` layer, resident when it fits the inline band — which
// nearly all of them do (measured: 55 of 58, median 1.1 KB). The rest need
// fetching, and that is what this is for. Pass nil when there is nothing to
// fetch with.
//
// Failures are reported and skipped rather than fatal: a catalog with one
// unparseable schema is still a catalog, and refusing to register the other
// two hundred would turn a single bad file into a dead universe. Every failure
// is logged loudly, because a schema that quietly fails to register becomes a
// URI that 404s later, somewhere that cannot explain why.
func RegisterDataTypes(
	ctx context.Context,
	svc api.SchemaService,
	cs []contribapi.Contribution,
	resolveSource func(uri string) ([]byte, error),
) (int, []DataTypeFailure) {
	registered := 0
	var failures []DataTypeFailure
	fail := func(uri, reason string) {
		failures = append(failures, DataTypeFailure{URI: uri, Reason: reason})
	}
	for _, dt := range cs {
		if dt.Type != contribapi.ResourceTypeDataType {
			continue
		}
		uri := dt.ResolvedURI()
		// The DSL is the source layer, and only there. It used to be wrapped
		// in a `{format, source}` body beside a source layer naming the same
		// bytes, so a reader had to know which of the two held the content.
		source := dt.Source()
		if len(source) == 0 && resolveSource != nil {
			if resolved, err := resolveSource(uri); err == nil {
				source = resolved
			}
		}
		if len(source) == 0 {
			// Distinguish "declares no schema" from "has one we could not
			// reach". The second becomes a URI that 404s later, somewhere that
			// cannot explain why — which is exactly what must not be silent.
			if _, declared := dt.SourceRef(); declared {
				slog.Warn("register data-type: schema content not reachable; this URI will 404 on GetDataType",
					"uri", uri)
				fail(uri, "schema content is declared but not reachable")
			}
			continue
		}
		if err := svc.RegisterDataType(ctx, api.RegisterDataTypeInput{
			URI: uri, Body: source,
		}); err != nil {
			slog.Error("register data-type FAILED — schema dropped from the registry; this URI will 404 on GetDataType",
				"uri", uri, "error", err,
				"hint", "run `schema parse <file>` to debug the schema body")
			fail(uri, err.Error())
			continue
		}
		// Loud SUCCESS log: every URI that survives registration should appear
		// here. A consumer that 404s on a URI which never printed this line was
		// never registered — which narrows the search to how the contribution
		// reached us, not to the parser.
		slog.Info("registered data-type", "uri", uri)
		registered++
	}
	return registered, failures
}

// BodyValidatorFor adapts a SchemaService into the framework's validation hook.
//
// The hook is a plain function rather than the service, which is what keeps the
// packages that only PUBLISH content from linking the parser: they take the
// same `ValidateContribution` and pass nil, and record that no validator was
// available instead of carrying one they never use.
func BodyValidatorFor(svc api.SchemaService) func(ctx context.Context, dataTypeURI string, body json.RawMessage) error {
	if svc == nil {
		return nil
	}
	return func(ctx context.Context, dataTypeURI string, body json.RawMessage) error {
		out, err := svc.ValidateAgainst(ctx, api.ValidateAgainstInput{
			URI: dataTypeURI, Body: body,
		})
		if err != nil {
			return err
		}
		if out.Valid && len(out.Diagnostics) == 0 {
			return nil
		}
		msgs := make([]string, 0, len(out.Diagnostics))
		for _, d := range out.Diagnostics {
			msgs = append(msgs, d.Message)
		}
		if len(msgs) == 0 {
			return fmt.Errorf("does not match %s", dataTypeURI)
		}
		return errors.New(strings.Join(msgs, "; "))
	}
}
