// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
)

type recipeBody struct {
	Title string   `json:"title" jsonschema:"The recipe's title"`
	Steps []string `json:"steps,omitempty"`
}

// A Go type's shape is published as an ordinary data-type of the provider:
// its source is a DSL document the schema parser reads back, whose root holds
// the struct's fields.
func TestGoTypeDataTypes_PublishTheShapeOfTheGoType(t *testing.T) {
	p := &GoTypeDataTypeProvider{ProviderName: "ktw-test",
		Types: []GoType{GoTypeOf[recipeBody]("recipe", "1.0.0", "Recipe", "A recipe.\nMore.")}}
	got, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want one data-type, got %d", len(got))
	}
	c := got[0]
	// The description is written once, in the document; the manifest builder
	// derives it into meta (the data-type kind's meta.from), never the producer.
	if c.ResolvedURI() != "ktw-test:/data-type/recipe" || c.Metadata.Description != "" {
		t.Errorf("entry %s %+v", c.ResolvedURI(), c.Metadata)
	}
	if d, err := contribapi.DeriveMeta(c, contribapi.LayerSource, []string{"description"}); err != nil || d.Description != "A recipe.\nMore." {
		t.Errorf("the document derives description %q (%v), want the type's", d.Description, err)
	}
	out, err := parser.Parse(context.Background(), schemaapi.ParseInput{Body: c.Source()})
	if err != nil {
		t.Fatalf("published document does not parse: %v", err)
	}
	root := out.Schema.Types[out.Schema.Root]
	if out.Schema.Root != "Recipe" || root == nil || root.Fields["title"] == nil || root.Fields["steps"] == nil {
		t.Fatalf("root %q fields %v, want Recipe with title and steps", out.Schema.Root, root)
	}
	if root.Fields["title"].Optional || !root.Fields["steps"].Optional {
		t.Error("title is required, steps optional (omitempty)")
	}
	if p.Type() != contribapi.ResourceTypeDataType {
		t.Errorf("Type = %q", p.Type())
	}
}
