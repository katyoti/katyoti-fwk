// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// projectsCWM answers as the store set of a universe serving several projects
// (cwm storeset.Served): ids answered bare, each read naming its project; a
// qualified id or parent reaches its key's project alone, a bare parent lists
// in every project, a bare id several projects hold is refused. It records
// whose verify hooks ran, by qualified id.
type projectsCWM struct {
	cwmapi.CWMService
	keys    []string
	objects map[string][]cwmapi.Object // by project key
	ran     []string
}

// at is the projects a reference reaches: its key's when qualified, else
// every project holding id (all of them for a list's parent).
func (c *projectsCWM) at(ref string, holding bool) ([]string, string) {
	if key, id, err := cwmapi.ParseQualifiedID(ref); err == nil {
		return []string{key}, id
	}
	if !holding {
		return c.keys, ref
	}
	var keys []string
	for _, k := range c.keys {
		for _, o := range c.objects[k] {
			if o.ID == ref {
				keys = append(keys, k)
				break
			}
		}
	}
	return keys, ref
}

func (c *projectsCWM) one(ref string) (string, cwmapi.Object, error) {
	keys, id := c.at(ref, true)
	if len(keys) > 1 {
		return "", cwmapi.Object{}, fmt.Errorf("%w: object %q is in several projects (%s)", cwmapi.ErrInvalidInput, id, strings.Join(keys, ", "))
	}
	for _, k := range keys {
		for _, o := range c.objects[k] {
			if o.ID == id {
				return k, o, nil
			}
		}
	}
	return "", cwmapi.Object{}, cwmapi.ObjectNotFound(ref)
}

func (c *projectsCWM) ObjectGet(_ context.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	key, o, err := c.one(in.ID)
	return cwmapi.ObjectGetOutput{Object: o, Project: key}, err
}

func (c *projectsCWM) ObjectList(_ context.Context, in cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	keys, parent := c.at(in.Parent, false)
	out := cwmapi.ObjectListOutput{}
	for _, k := range keys {
		for _, o := range c.objects[k] {
			if o.Parent == parent {
				out.Objects = append(out.Objects, o)
				out.Projects = append(out.Projects, k)
			}
		}
	}
	return out, nil
}

func (c *projectsCWM) ObjectRunHooks(_ context.Context, in cwmapi.ObjectRunHooksInput) (cwmapi.ObjectRunHooksOutput, error) {
	key, o, err := c.one(in.ID)
	if err != nil {
		return cwmapi.ObjectRunHooksOutput{}, err
	}
	c.ran = append(c.ran, cwmapi.QualifiedID(key, o.ID))
	if _, ok := o.Capability(cwmapi.CapCheckable); !ok {
		return cwmapi.ObjectRunHooksOutput{}, nil
	}
	status := "pass"
	if key == "a" {
		status = "fail" // the homonym's criteria fail: reaching them shows
	}
	return cwmapi.ObjectRunHooksOutput{Objects: 1, Runs: []cwmapi.HookRun{{ObjectID: o.ID, URI: "probe", Answer: []byte(`{"status":"` + status + `"}`)}}}, nil
}

// : Verify of another project's object, named by its
// qualified id, walks that project's subtree alone - its children listed and
// its hooks run by qualified id - never a homonym's in another project.
func TestVerifyOfAQualifiedIdWalksOnlyItsProjectsSubtree(t *testing.T) {
	cwm := &projectsCWM{keys: []string{"a", "b"}, objects: map[string][]cwmapi.Object{
		"a": {{ID: "m.F-1", Type: "feature"}, criterion("m.AC-1", "bash a.sh"), criterion("m.AC-9", "bash a9.sh")},
		"b": {{ID: "m.F-1", Type: "feature"}, criterion("m.AC-1", "bash b.sh"), criterion("m.AC-2", "bash b2.sh")},
	}}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		application.DeclareSimpleService[cwmapi.CWMService](func(context.Context) (cwmapi.CWMService, error) { return cwm, nil }))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	ctx := application.WithApplication(context.Background(), app)

	out, err := (&Service{}).Verify(ctx, api.VerifyInput{ID: "b:m.F-1"})
	if err != nil {
		t.Fatalf("verify of b:m.F-1: %v", err)
	}
	want := []string{"b:m.F-1", "b:m.AC-1", "b:m.AC-2"}
	if !slices.Equal(cwm.ran, want) {
		t.Errorf("hooks ran on %v, want %v (project b's subtree alone)", cwm.ran, want)
	}
	if out.Runs != 2 || out.Passed != 2 || len(out.Failed) != 0 {
		t.Errorf("verify: %+v (want 2 runs, both passing - none of project a's)", out)
	}
}
