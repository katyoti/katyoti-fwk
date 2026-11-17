// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

const testPID = "00000000-0000-7000-8000-000000000001"

// writeProjectObject gives a folder a root project object with the given id.
func writeProjectObject(t *testing.T, dir, name, uuid string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "id: " + name + "\ntype: project\ncapabilities:\n  container:\n    name: " + name + "\n  project:\n    uuid: " + uuid + "\n"
	if err := os.WriteFile(filepath.Join(dir, "project.cwm.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// RESOLVE: a full name from a log names the universe and folder that mount
// its project, and hands the authored id back for the core to finish.
func TestResolveProjectFindsTheMount(t *testing.T) {
	root := projectTestRoot(t)
	seedUniverseFor(t, root, "acme")
	dir := filepath.Join(root, "checkout")
	writeProjectObject(t, dir, "ktw-acme-demo", testPID)
	s := &Service{}
	if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: "acme", ProjectDir: dir}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	out, err := s.ResolveProject(context.Background(), api.ResolveProjectInput{Ref: testPID + ".live.g1"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if out.Universe != "acme" || out.Name != "ktw-acme-demo" || out.ObjectID != "live.g1" || out.ProjectID != testPID {
		t.Errorf("resolve = %+v", out)
	}
	if abs, _ := filepath.Abs(dir); out.ProjectDir != abs {
		t.Errorf("project dir = %q, want %q", out.ProjectDir, abs)
	}
	if _, err := s.ResolveProject(context.Background(), api.ResolveProjectInput{Ref: "01a0d3a2-0000-7000-8000-000000000000"}); err == nil || !strings.Contains(err.Error(), "not mounted") {
		t.Errorf("an unmounted id must say so: %v", err)
	}
}

// THE SAME PROJECT ID TWICE IN ONE UNIVERSE IS REFUSED, naming the existing
// mount: a universe mounts several repositories, each a project of its own
// key, so a second folder of one key would be one project served twice.
func TestAttachProjectRefusesTheSameProjectIDTwice(t *testing.T) {
	root := projectTestRoot(t)
	seedUniverseFor(t, root, "acme")
	a := filepath.Join(root, "clone-a")
	b := filepath.Join(root, "clone-b")
	writeProjectObject(t, a, "demo", testPID)
	writeProjectObject(t, b, "demo", testPID)
	markCreated(t, a, b) // a second member is one created on this PC
	s := &Service{}
	if _, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: "acme", ProjectDir: a}); err != nil {
		t.Fatalf("attach a: %v", err)
	}
	_, err := s.AttachProject(context.Background(), api.AttachProjectInput{Universe: "acme", ProjectDir: b})
	if err == nil {
		t.Fatal("the same project id from a second folder was accepted")
	}
	if !strings.Contains(err.Error(), `"demo"`) || !strings.Contains(err.Error(), a) {
		t.Errorf("the refusal must name the project and the existing mount: %v", err)
	}
}
