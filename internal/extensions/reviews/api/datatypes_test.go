// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

const regen = "run `KTW_REGEN_DATATYPES=1 go test ./internal/extensions/reviews/api/ -run TestReviewsDataTypes`"

// reviewsDataTypes are the data-types of the reviews pack the service decodes
// with a Go type, rendered from that type into contrib/embed/cwm/data-types/ —
// so the form, CWM's check and the service read one declaration.
var reviewsDataTypes = []struct {
	folder string
	src    datatypegen.Source
}{
	{"", datatypegen.Source{
		Name: "review", Version: "1.0.0", Root: "Review",
		Origin:      "api.ReviewFields (internal/extensions/reviews/api/reviews.go)",
		Description: "The review type's own fields: the review type it ran, what it looked at, when,\non which commit, what triggered it, who reviewed, its rating, score and summary.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.ReviewFields]() },
	}},
	{"", datatypegen.Source{
		Name: "finding", Version: "1.0.0", Root: "Finding",
		Origin:      "api.FindingFields (internal/extensions/reviews/api/reviews.go)",
		Description: "The finding type's own fields: its severity, what was found, where, what to\ndo about it, the review type that found it and its fingerprint.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.FindingFields]() },
	}},
	{"", datatypegen.Source{
		Name: "cap-reviewable", Version: "1.0.0", Root: "CapReviewable",
		Origin:      "api.ReviewableFields (internal/extensions/reviews/api/history.go)",
		Description: "Field-group for the `reviewable` capability: the compact history of the reviews\nfolded away, one entry each.\n",
		AST:         func() (*schemaapi.Type, error) { return ktwreflect.TypeAST[api.ReviewableFields]() },
	}},
}

// TestReviewsDataTypesAreCurrent refuses a stale pack data-type.
func TestReviewsDataTypesAreCurrent(t *testing.T) {
	dir := filepath.Join("..", "contrib", "embed", "cwm", "data-types")
	for _, d := range reviewsDataTypes {
		want, err := datatypegen.RenderAs(d.src, datatypegen.Header{
			Tool: "TestReviewsDataTypesAreCurrent", Regenerate: regen, URI: "ktw-agents:/data-type/" + d.src.Name,
		})
		if err != nil {
			t.Fatalf("%s: %v", d.src.Name, err)
		}
		path := filepath.Join(dir, d.folder, d.src.Name+".dsl.yaml")
		if os.Getenv("KTW_REGEN_DATATYPES") == "1" {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (%s)", err, regen)
		}
		if strings.ReplaceAll(string(got), "\r\n", "\n") != string(want) {
			t.Errorf("%s is stale — the Go type is the source; %s", path, regen)
		}
	}
}
