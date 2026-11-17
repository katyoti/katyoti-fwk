// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// countingSchemaStub is validatorStub that counts how often each data-type
// is resolved.
type countingSchemaStub struct {
	validatorStub
	resolved map[string]*atomic.Int32
}

func (s *countingSchemaStub) GetDataTypeByURI(ctx context.Context, uri string) (json.RawMessage, error) {
	if s.resolved[uri] == nil {
		s.resolved[uri] = &atomic.Int32{}
	}
	s.resolved[uri].Add(1)
	return s.validatorStub.GetDataTypeByURI(ctx, uri)
}

// A reindex resolves each data-type once, however many entries its kind
// has: resolving decodes the whole schema document, and it used to be done
// per entry — hundreds of times over a handful of data-types, twice at
// every hub boot.
func TestReindex_ResolvesEachDataTypeOnce(t *testing.T) {
	stub := &countingSchemaStub{resolved: map[string]*atomic.Int32{}}
	entries := []contribapi.Contribution{kindDecl("skill", "system:/data-type/skill")}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		entries = append(entries, contribapi.Contribution{Provider: "test", Type: "skill", ID: id,
			Layers: contribapi.InlineData(json.RawMessage(`{"name":"` + id + `"}`))})
	}
	cat := seedCatalog(t, contribapi.ContributionsConfig{}, &stub.validatorStub, entries)
	// seedCatalog reindexed with the plain stub; reindex again with the
	// counting one, which is what this test reads.
	if err := cat.SetSchema(stub); err != nil {
		t.Fatal(err)
	}
	if err := cat.Reindex(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	n := stub.resolved["system:/data-type/skill"]
	if n == nil {
		t.Fatal("the skill data-type was never resolved: edge extraction did not run")
	}
	if got := n.Load(); got != 1 {
		t.Errorf("system:/data-type/skill resolved %d times in one reindex, want 1", got)
	}
}
