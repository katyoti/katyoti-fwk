// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// TestValidate_OrphanLinkTarget rejects a link to an undeclared type.
func TestValidate_OrphanLinkTarget(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: orphan
types:
  Post:
    type: object
    fields:
      author: ->User
`
	_, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err == nil {
		t.Fatal("expected validation error for orphan link target")
	}
	if !strings.Contains(err.Error(), "User") {
		t.Errorf("error should mention orphan target name 'User': %v", err)
	}
}

// TestValidate_OrphanArrayItem rejects an array of an undeclared type.
func TestValidate_OrphanArrayItem(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: orphan
types:
  Post:
    type: object
    fields:
      tags: Tag[]
`
	_, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err == nil {
		t.Fatal("expected validation error for orphan array item")
	}
}

// TestValidate_OrphanExtends rejects extends pointing to an undeclared type.
func TestValidate_OrphanExtends(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: orphan
types:
  Article:
    type: object
    extends: NoSuchBase
    fields:
      title: string
`
	_, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err == nil {
		t.Fatal("expected validation error for orphan extends")
	}
}

// TestValidate_UnknownContentIsAccepted: an unknown content type parses with a
// WARNING, not an error. Content/field types are defined by the ktw-display-lib
// field-type manifests and are extensible — the Go parser must not restrict.
func TestValidate_BadContent(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: bad
types:
  Field:
    type: string@nonsense
`
	_, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err != nil {
		t.Fatalf("unknown content type must parse (warn, not error) — the parser does not restrict; got: %v", err)
	}
}

// TestValidate_ContentMismatch: a content type not in the common set for the
// kind (e.g. @email on number) parses with a warning — not rejected. Authority
// is the display library, not a hardcoded Go list.
func TestValidate_ContentMismatch(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: bad
types:
  Field:
    type: number@email
`
	_, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err != nil {
		t.Fatalf("mismatched content type must parse (warn, not error); got: %v", err)
	}
}

// TestValidate_LinkNoTarget rejects a link without a target.
func TestValidate_LinkNoTarget(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: bad
types:
  Field:
    type: link
`
	_, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err == nil {
		t.Fatal("expected validation error for link without target")
	}
}

// TestValidate_OrphanRoot rejects root pointing to a missing type.
func TestValidate_OrphanRoot(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: bad
root: NoSuchType
types:
  Other:
    type: object
`
	_, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err == nil {
		t.Fatal("expected validation error for orphan root")
	}
}

// TestValidate_ValidSchema confirms a properly-typed schema passes.
func TestValidate_ValidSchema(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: ok
root: User
types:
  User:
    type: object
    id: id
    fields:
      id: string@uuid
      email: string@email?
      name: string@text(max:100)
      manager: ->User
      tags: string[]
`
	out, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, d := range out.Diagnostics {
		if d.Severity == schemaapi.SeverityError {
			t.Errorf("unexpected error diagnostic: %+v", d)
		}
	}
	if out.Schema.Types["User"] == nil {
		t.Fatal("User type missing")
	}
}

// TestValidate_TreeOverridesAreWarnings ensures explicit tree.mode/linkType
// produce warnings, not errors, since the tree machinery auto-corrects them.
func TestValidate_TreeOverridesAreWarnings(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: tree-test
types:
  Category:
    type: object
    fields:
      parent:
        type: tree
        target: Category
        mode: many2many
        link_type: associated
`
	out, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err != nil {
		t.Fatalf("Parse should not error on tree override: %v", err)
	}
	hasWarnings := false
	for _, d := range out.Diagnostics {
		if d.Severity == schemaapi.SeverityWarning && strings.Contains(d.Code, "TREE_") {
			hasWarnings = true
		}
	}
	if !hasWarnings {
		t.Errorf("expected TREE_* warnings, got: %+v", out.Diagnostics)
	}
}
