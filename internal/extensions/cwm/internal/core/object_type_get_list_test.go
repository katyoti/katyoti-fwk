// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"sort"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE TYPE VERBS the forms read: ObjectTypeList lists every declared type,
// sorted by name; ObjectTypeGet answers one, an undeclared name is not found,
// and no name at all is refused.

func TestObjectTypeListListsTheDeclaredTypesSorted(t *testing.T) {
	svc, _ := namedFileFixture(t) // declares doc and plain
	out, err := svc.ObjectTypeList(stdctx.Background(), api.ObjectTypeListInput{})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, ot := range out.ObjectTypes {
		names = append(names, ot.Name)
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("not sorted by name: %v", names)
	}
	for _, want := range []string{"doc", "plain"} {
		if !hasName(names, want) {
			t.Errorf("declared type %q missing from %v", want, names)
		}
	}
	if len(dedupe(append([]string(nil), names...))) != len(names) {
		t.Errorf("a type listed twice: %v", names)
	}
}

func TestObjectTypeListOfAnUndeclaredCatalogIsEmptyNotNil(t *testing.T) {
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	out, err := svc.ObjectTypeList(stdctx.Background(), api.ObjectTypeListInput{})
	if err != nil || out.ObjectTypes == nil {
		t.Fatalf("list: %#v %v, want an empty (non-nil) list", out.ObjectTypes, err)
	}
	for _, ot := range out.ObjectTypes {
		if ot.Name == "doc" || ot.Name == "plain" {
			t.Errorf("a type nobody declared here: %q", ot.Name)
		}
	}
}

func TestObjectTypeGetAnswersOneDeclaredType(t *testing.T) {
	svc, _ := namedFileFixture(t)
	ctx := stdctx.Background()
	got, err := svc.ObjectTypeGet(ctx, api.ObjectTypeGetInput{Name: "doc"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ObjectType.Name != "doc" || !hasName(got.ObjectType.Capabilities, "memory") {
		t.Errorf("doc = %+v, want its declaration (capabilities incl. memory)", got.ObjectType)
	}
	if _, err := svc.ObjectTypeGet(ctx, api.ObjectTypeGetInput{Name: "nope"}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("an undeclared type: %v, want ErrNotFound", err)
	}
	if _, err := svc.ObjectTypeGet(ctx, api.ObjectTypeGetInput{}); !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("no name: %v, want ErrInvalidInput", err)
	}
}
