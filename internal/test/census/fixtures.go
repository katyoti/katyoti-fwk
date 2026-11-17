// SPDX-License-Identifier: Apache-2.0

package census

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// fixturePacksDir is where the contribution fixtures live, from the repo root.
var fixturePacksDir = filepath.Join("internal", "test", "fixtures", "contributions")

// FixtureDeclarations reads the kind declarations and schemas of the named
// fixture packs (`internal/test/fixtures/contributions/<pack>`): every
// `contribution-types/*.yaml` and `data-types/*.yaml`, as the pack's host
// would publish them — through the same producers, so a declaration the
// census resolves a foreign kind against is exactly the one the universe
// resolves it against. The provider is the pack's folder name.
//
// This is how the census names the DECLARED universe a binary's foreign
// kinds (Manifest.ForeignKinds) must resolve in: AssertValidWithin(t, name,
// m, svc, FixtureDeclarations(t, "files-only-pack")). A binary that names no
// pack has no foreign kind to resolve, and one it cannot resolve fails.
func FixtureDeclarations(t *testing.T, packs ...string) []contribapi.Contribution {
	t.Helper()
	var out []contribapi.Contribution
	for _, pack := range packs {
		out = append(out, DeclarationsOf(t, pack, filepath.Join(fixturePacksDir, pack))...)
	}
	return out
}

// DeclarationsOf reads the kind declarations and schemas a universe member
// publishes from `dir` (from the repo root) — its `contribution-types/` and
// `data-types/` folders, through the same producers, under `provider`. Point
// it at the member's real embed folder (`app/ktw-docs/embed`) so the census
// judges a foreign kind's entries by the declaration the universe will
// actually hold.
func DeclarationsOf(t *testing.T, provider, dir string) []contribapi.Contribution {
	t.Helper()
	folder := filepath.Join(repoRoot(t), dir)
	if fi, err := os.Stat(folder); err != nil || !fi.IsDir() {
		t.Fatalf("census: declarations of %q: %s is not a folder", provider, dir)
	}
	fsl := loader.NewFSLoader(folder)
	var out []contribapi.Contribution
	for _, p := range []contribapi.ContributionProvider{
		manifestcontrib.NewContributionTypeProvider(provider, fsl, "contribution-types"),
		manifestcontrib.NewDataTypeProvider(provider, fsl, "data-types"),
	} {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatalf("census: read %s of %q: %v", p.Type(), provider, err)
		}
		for i := range cs {
			if cs[i].URI == "" {
				cs[i].URI = cs[i].ResolvedURI()
			}
		}
		out = append(out, cs...)
	}
	return out
}

// LeafDeclarationsOf is the kind declarations and schemas a universe member
// publishes for the built-in leaves it carries (builtin.ContributeLeafKinds),
// under provider, through the same publisher: the declarations the census
// judges that member's leaf kinds' entries against.
func LeafDeclarationsOf(t *testing.T, provider string, leaves ...leaf.Declaration) []contribapi.Contribution {
	t.Helper()
	var out []contribapi.Contribution
	for _, p := range builtin.Providers(provider, leaves...) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatalf("census: read %s of %q: %v", p.Type(), provider, err)
		}
		for i := range cs {
			if cs[i].URI == "" {
				cs[i].URI = cs[i].ResolvedURI()
			}
		}
		out = append(out, cs...)
	}
	return out
}

// repoRoot walks up from the working directory to the folder holding go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("census: cwd: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("census: no go.mod above %s", dir)
	return ""
}
