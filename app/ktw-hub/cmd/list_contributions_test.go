// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// listFixture: five skills, each with a control-band data layer (bytes the
// index form drops), in one provider.
func listFixture() *catalogServiceImpl {
	big := bytes.Repeat([]byte("x"), 8<<10)
	var cs []contribapi.Contribution
	for i := 0; i < 5; i++ {
		c := contribapi.Contribution{Provider: "ktw-docs", Type: "skill", ID: fmt.Sprintf("s%d", i)}
		c.SetData(json.RawMessage(`"` + string(big) + `"`))
		cs = append(cs, c)
	}
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{{Provider: "ktw-docs", Contributions: cs}}, nil)
	return &catalogServiceImpl{cat: cat}
}

// ListContributions is the index lane: it pages like ListResources, answers in
// index form whether paged or not, and its wire key is "contributions".
func TestListContributions_PagesInIndexFormUnderItsOwnKey(t *testing.T) {
	ctx := context.Background()
	s := listFixture()

	var got []contribapi.Contribution
	cursor := ""
	for page := 0; page < 10; page++ {
		out, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if out.Pagination.Total != 5 || out.Pagination.PageSize != len(out.Contributions) {
			t.Fatalf("page %d pagination = %+v", page, out.Pagination)
		}
		got = append(got, out.Contributions...)
		if !out.Pagination.HasMore {
			break
		}
		cursor = out.Pagination.NextCursor
	}
	if len(got) != 5 {
		t.Fatalf("drained %d entries, want 5", len(got))
	}

	for _, in := range []mbapi.CatalogListContributionsInput{{Limit: 2}, {}} { // paged and unpaged
		out, err := s.ListContributions(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range out.Contributions {
			if len(c.Data()) != 0 || c.DataHash() == "" {
				t.Fatalf("%+v: %s carries %d data bytes (hash %q), want the index form", in, c.ResolvedURI(), len(c.Data()), c.DataHash())
			}
		}
		raw, _ := json.Marshal(out)
		var keys map[string]json.RawMessage
		_ = json.Unmarshal(raw, &keys)
		if _, ok := keys["contributions"]; !ok {
			t.Fatalf("wire keys %v, want \"contributions\"", keys)
		}
		if _, ok := keys["resources"]; ok {
			t.Fatal("the new method must not answer under the compatibility key \"resources\"")
		}
	}

	// Filter only (no legacy Type): a filter that matches nothing answers
	// nothing.
	out, err := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{Filter: contribapi.ListFilter{Types: []string{"task"}}})
	if err != nil || len(out.Contributions) != 0 {
		t.Fatalf("Types=[task]: %d entries, %v", len(out.Contributions), err)
	}
}

// ListResources is the compatibility adapter over the same code path: the same
// entries, under "resources", with bodies when unpaginated (its legacy default)
// or when IncludeBody asks.
func TestListResources_IsTheCompatibilityAdapter(t *testing.T) {
	ctx := context.Background()
	s := listFixture()

	idx, _ := s.ListContributions(ctx, mbapi.CatalogListContributionsInput{Limit: 3})
	compat, err := s.ListResources(ctx, mbapi.CatalogListResourcesInput{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Contributions) != len(compat.Contributions) || idx.Pagination != compat.Pagination {
		t.Fatalf("compat page %+v differs from the index page %+v", compat.Pagination, idx.Pagination)
	}
	for i := range idx.Contributions {
		if idx.Contributions[i].ResolvedURI() != compat.Contributions[i].ResolvedURI() || len(compat.Contributions[i].Data()) != 0 {
			t.Fatalf("entry %d: compat %s differs from index %s", i, compat.Contributions[i].ResolvedURI(), idx.Contributions[i].ResolvedURI())
		}
	}

	for _, in := range []mbapi.CatalogListResourcesInput{{}, {Limit: 3, IncludeBody: true}} {
		out, err := s.ListResources(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range out.Contributions {
			if len(c.Data()) == 0 {
				t.Fatalf("%+v: %s has no body, the compatibility method keeps it", in, c.ResolvedURI())
			}
		}
		raw, _ := json.Marshal(out)
		if !bytes.Contains(raw, []byte(`"resources":`)) {
			t.Fatal("the compatibility output must keep its wire key \"resources\"")
		}
	}
}
