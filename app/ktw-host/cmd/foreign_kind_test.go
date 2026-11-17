// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

// foreignKindChildEnv marks the child process TestHostServesAKindAnotherMemberDeclares
// re-executes itself in.
const foreignKindChildEnv = "KTW_TEST_FOREIGN_KIND_CHILD"

// foreignKindPack is the e29 fixture: a pack that publishes entries of a kind
// it does not declare (`recipe`, declared by files-only-pack) through a
// `dirs:` line and no `contribution-types/` folder.
func foreignKindPack(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(here), "..", "..", "..",
		"internal", "test", "fixtures", "contributions", "foreign-kind-pack")
}

// TestHostServesAKindAnotherMemberDeclares is the in-process half of blind
// audit H3 for a hosted `dirs:` pack: the generic host, pointed at a folder
// whose kind is declared by ANOTHER member, still builds its manifest — the
// kind is recorded as foreign, the entry is kept — and the entry passes the
// census once the census is told which pack declares the kind, judged by that
// pack's declaration and schema. Told nothing, the census refuses it with the
// kind and the URI: the strict posture the aggregator applies at runtime.
// Before the fix the build failed: `undeclared contribution kind(s) [recipe]`.
// The cross-process half, through a hub, is
// internal/test/catalog.TestForeignKindThroughTheUniverse.
func TestHostServesAKindAnotherMemberDeclares(t *testing.T) {
	// ONE application per process (census.AssertApp): the files-only test in
	// this package already built the host once, and its schema check
	// registered into the process-wide points. This case runs itself in a
	// child process so it constructs the host first, alone.
	if os.Getenv(foreignKindChildEnv) == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v", "-test.count=1")
		cmd.Env = append(os.Environ(), foreignKindChildEnv+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("child process failed: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "--- PASS: "+t.Name()) {
			t.Fatalf("child process did not report a pass:\n%s", out)
		}
		return
	}
	census.Isolate(t)
	t.Setenv(contentRootEnv, foreignKindPack(t))

	app := application.New(application.Config(), buildExtensions(embed.FS{})...)
	if err := app.Run(); err != nil {
		t.Fatalf("run host over the fixture: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)
	m, err := manifestlocal.NewBuilder("foreign-kind-pack", "1.0.0").Build(ctx)
	if err != nil {
		t.Fatalf("build manifest: %v — a kind another member declares must not fail the build", err)
	}

	const entryURI = "foreign-kind-pack:/recipe/waffles"
	var entry *contribapi.Contribution
	for i := range m.Contributions {
		if m.Contributions[i].ResolvedURI() == entryURI {
			entry = &m.Contributions[i]
		}
		if m.Contributions[i].Type == contribapi.ResourceTypeContributionType && m.Contributions[i].ID == "recipe" {
			t.Fatalf("the fixture must not declare recipe itself: %s", m.Contributions[i].ResolvedURI())
		}
	}
	if entry == nil {
		t.Fatalf("host did not publish %s (%d contributions)", entryURI, len(m.Contributions))
	}
	if len(m.ForeignKinds) != 1 || m.ForeignKinds[0] != "recipe" {
		t.Errorf("ForeignKinds = %v, want [recipe]", m.ForeignKinds)
	}
	if _, declared := contribapi.KindsFrom(m.Contributions).Get("recipe"); declared {
		t.Errorf("recipe reads as declared by this binary; it is not")
	}

	// Strict: against the declaring pack the census passes and the entry is
	// really validated against files-only-pack's schema (a budget, required
	// layers and a body check it could not have run without the declaration).
	svc := census.SchemaService(t)
	census.AssertValidWithin(t, "foreign-kind-pack", m, svc, census.FixtureDeclarations(t, "files-only-pack"))
	if t.Failed() {
		return
	}
	// Told nothing, the census refuses the entry — with the kind and the URI.
	// Falsified through the decision behind AssertValidWithin rather than a
	// failing t: the message is what an author would read.
	refusal := census.UnresolvedForeignKinds(m, nil)
	if len(refusal) != 1 {
		t.Fatalf("with no declaring pack named, want one refusal, got %v", refusal)
	}
	for _, frag := range []string{entryURI, `kind "recipe"`, "neither foreign-kind-pack nor system"} {
		if !strings.Contains(refusal[0], frag) {
			t.Errorf("refusal %q lacks %q", refusal[0], frag)
		}
	}
}
