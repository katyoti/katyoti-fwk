// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/frozenhost"
	"github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
)

// The contributor this serves has FILES and no Go toolchain. Scaffolding has to
// produce a folder that is immediately valid — otherwise their first step is
// reading documentation to hand-write a manifest, which is a poor opening for a
// feature whose pitch is "just write files".
func TestCreateContent_ProducesAPublishableFolder(t *testing.T) {
	dir := t.TempDir()
	out, err := (&Service{}).CreateContent(context.Background(), api.CreateContentInput{
		Name:        "team-playbooks",
		Out:         dir,
		Description: "Our team's playbooks",
	})
	if err != nil {
		t.Fatalf("CreateContent: %v", err)
	}

	// frozenhost.ReadIdentity is what BOTH ktw-host and the publisher use, so
	// passing it means the folder runs AND publishes — not merely that some
	// files exist.
	id, err := frozenhost.ReadIdentity(out.Folder)
	if err != nil {
		t.Fatalf("the scaffolded folder is not a valid content folder: %v", err)
	}
	if id.Name != "team-playbooks" || id.Description != "Our team's playbooks" {
		t.Errorf("identity not carried into the manifest: %+v", id)
	}

	// An included dir with no valid file fails the strict skill-corpus gate, so
	// an empty skills/ would hand the user a folder that refuses to publish.
	skill := filepath.Join(out.Folder, "skills", "team-playbooks-usage.md")
	body, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("no starter skill: %v", err)
	}
	if !strings.HasPrefix(string(body), "---") {
		t.Error("starter skill has no frontmatter — the skill gate would reject it")
	}
}

// compose never overwrites. Scaffolding onto an existing folder would destroy
// work that is, by definition, not in a build system that could restore it.
func TestCreateContent_RefusesAnExistingFolder(t *testing.T) {
	dir := t.TempDir()
	in := api.CreateContentInput{Name: "acme", Out: dir}
	if _, err := (&Service{}).CreateContent(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Service{}).CreateContent(context.Background(), in); err == nil {
		t.Error("scaffolded over an existing folder")
	}
}

// The name becomes the catalog item AND the runtime provider, both of which are
// kebab tokens. Catching it here beats a confusing failure at publish time.
func TestCreateContent_RefusesANonKebabName(t *testing.T) {
	_, err := (&Service{}).CreateContent(context.Background(), api.CreateContentInput{
		Name: "Team Playbooks", Out: t.TempDir(),
	})
	if err == nil {
		t.Fatal("accepted a non-kebab name")
	}
	if !strings.Contains(err.Error(), "kebab") {
		t.Errorf("error %q does not say what is wrong with the name", err)
	}
}
