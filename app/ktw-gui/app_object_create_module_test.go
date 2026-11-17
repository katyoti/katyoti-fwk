// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// : a module « + New » creates under the project root is a
// module - its id its own bare name, its own folder - and the objects tree
// lists it once read again. The binding used to file it in the parent's
// module with a minted id (project.<uuid>): an object of the project root
// typed software-module, which WorkspaceObjectsTree never served.

// cwmSender answers the binding's cwm calls from a real CWMService - the
// wire's JSON round trip kept, no stubbed answer.
type cwmSender struct {
	ctx context.Context
	cwm cwmapi.CWMService
}

func (s cwmSender) Call(_ context.Context, path string, params any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	var out any
	var err error
	switch path {
	case "ktw-agents:cwm/object-create":
		out, err = s.cwm.ObjectCreate(s.ctx, params.(cwmapi.ObjectCreateInput))
	case "ktw-agents:cwm/object-tree":
		out, err = s.cwm.ObjectTree(s.ctx, params.(cwmapi.ObjectTreeInput))
	case "ktw-agents:cwm/object-type-get":
		out, err = s.cwm.ObjectTypeGet(s.ctx, params.(cwmapi.ObjectTypeGetInput))
	default:
		return nil, fmt.Errorf("cwmSender: no route for %q", path)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (cwmSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

func TestAModuleCreatedUnderTheProjectRootIsListedByTheTree(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.SoftwareModules(t))
	if _, err := cwm.ProjectInit(ctx, cwmapi.ProjectInitInput{Name: "project"}); err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	s := cwmSender{ctx: ctx, cwm: cwm}

	made, err := objectCreateVia(ctx, s, "project", "software-module", "", "Walk module 1794313660851", "a module taking a use case", "{}")
	if err != nil {
		t.Fatalf("objectCreateVia: %v", err)
	}
	if made.Object.ID != "walk-module-1794313660851" || made.Object.Module != made.Object.ID {
		t.Errorf("created %q in module %q, want the module walk-module-1794313660851, its own module", made.Object.ID, made.Object.Module)
	}

	rows, err := objectsTreeVia(ctx, s)
	if err != nil {
		t.Fatalf("objectsTreeVia: %v", err)
	}
	var listed bool
	for _, r := range rows {
		listed = listed || r.ID == made.Object.ID
	}
	if !listed {
		t.Fatalf("WorkspaceObjectsTree does not list the module %q just created under the project root: %+v", made.Object.ID, rows)
	}

	// A use case under it lands in the new module.
	uc, err := objectCreateVia(ctx, s, made.Object.ID, "feature", "", "A feature", "", "")
	if err != nil {
		t.Fatalf("create under the module: %v", err)
	}
	if uc.Object.Module != made.Object.ID {
		t.Errorf("a child of the module is filed in %q, want %q", uc.Object.Module, made.Object.ID)
	}
}

// : an accented module name gets a readable id - its letters
// transliterated, never dropped - at most cwmapi.SlugMax long; a type CWM
// cannot read is refused with its reason, never created as a plain object.
func TestAnAccentedModuleNameGetsAReadableID(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.SoftwareModules(t))
	if _, err := cwm.ProjectInit(ctx, cwmapi.ProjectInitInput{Name: "project"}); err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	s := cwmSender{ctx: ctx, cwm: cwm}

	for name, want := range map[string]string{
		"Études été": "etudes-ete",
		"Une très longue étude des écarts entre les applications métier": "une-tres-longue-etude-des-ecarts-entre-les",
	} {
		made, err := objectCreateVia(ctx, s, "project", "software-module", "", name, "", "{}")
		if err != nil {
			t.Fatalf("objectCreateVia(%q): %v", name, err)
		}
		if made.Object.ID != want {
			t.Errorf("module %q got the id %q, want %q", name, made.Object.ID, want)
		}
		if len(made.Object.ID) > cwmapi.SlugMax {
			t.Errorf("module id %q is over %d", made.Object.ID, cwmapi.SlugMax)
		}
	}

	// A type read that fails for another reason than an undeclared type is
	// refused, never taken for « not a module ».
	if _, err := objectCreateVia(ctx, typeReadFails{s}, "project", "software-module", "", "Anything", "", "{}"); err == nil || !strings.Contains(err.Error(), "type registry down") {
		t.Errorf("a failed type read gave %v, want its error", err)
	}
}

// typeReadFails is a sender whose type reads fail.
type typeReadFails struct{ cwmSender }

func (f typeReadFails) Call(ctx context.Context, path string, params any, opts ...ktsapi.CallOption) (json.RawMessage, error) {
	if path == "ktw-agents:cwm/object-type-get" {
		return nil, errors.New("type registry down")
	}
	return f.cwmSender.Call(ctx, path, params, opts...)
}
