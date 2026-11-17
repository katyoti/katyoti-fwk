// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// : PLANNING BEHAVES THE SAME ON A CONVERTED REPOSITORY. A
// repository as it stood before its conversion (its modules typed module, its
// project typed project) and the same repository converted by the code's
// migration steps, each read by the same CWM with every pack's types, hooks
// and policies, answer the same for every object: the capabilities in force
// (what planification, the scheduler, the panels and the reviews ask), the
// hooks that fire (planning a module, a request), and - under every module
// and project - which types may be written there and which « + New » offers.
// Both copies are made from git; the repository itself is never written.

// convertedRepositories are the repositories proven, each at a commit before
// its conversion.
var convertedRepositories = []struct{ name, dir, commit string }{
	{"ktw-agents", "", "ee6e22086^"}, // the last commit before converted this workspace
}

// typeNameAcceptParents are the types a converted module no longer offers in
// « + New » because their accept_parents names a type that is gone, each with
// the request that settles it. Empty since: an entry naming the
// module level is a capability the converted module's type carries.
var typeNameAcceptParents = map[string]string{}

// knownCapabilityChanges are the objects whose capabilities the conversion
// changes, each with the request that settles it.
var knownCapabilityChanges = map[string]string{
	"drafts": "a module not planned becomes documentation, which is planned",
}

// codeOfDocumentation are the capabilities a documentation module leaves
// behind once converted (data version 29): it holds no code, so
// neither the code capability nor module-health, the code checks attached to
// it, stay in force on it.
var codeOfDocumentation = []string{"code", "module-health"}

// withoutCodeOfDocumentation is what an object answered before its
// conversion, as its converted type expects it: a documentation module's
// code capabilities taken out, any other object's left as they were.
func withoutCodeOfDocumentation(after behaviour, before []string) []string {
	if after.typ != "documentation" {
		return before
	}
	return slices.DeleteFunc(slices.Clone(before), func(c string) bool { return slices.Contains(codeOfDocumentation, c) })
}

// behaviourConfig is the corpus configuration with what decides behaviour
// beside the types: the packs' hooks, hook types and policies.
func behaviourConfig(t *testing.T) (*CWMServiceConfig, string) {
	t.Helper()
	cfg, up := realCorpusConfig(t)
	packs, _ := filepath.Glob(filepath.Join(up, "app", "*", "embed", "cwm"))
	more, _ := filepath.Glob(filepath.Join(up, "internal", "extensions", "*", "contrib", "embed", "cwm"))
	seenHook, seenPoint, seenPolicy := map[string]bool{}, map[api.HookPoint]bool{}, map[string]bool{}
	for _, dir := range append(packs, more...) {
		fsys := os.DirFS(dir)
		hooks, err := storage.LoadHooksLayered(fsys)
		if err != nil {
			t.Fatalf("load hooks %s: %v", dir, err)
		}
		for _, h := range hooks {
			if !seenHook[h.Name] {
				seenHook[h.Name] = true
				cfg.CatalogTypes.Hooks = append(cfg.CatalogTypes.Hooks, h)
			}
		}
		points, err := storage.LoadHookTypesLayered(fsys)
		if err != nil {
			t.Fatalf("load hook types %s: %v", dir, err)
		}
		for _, p := range points {
			if !seenPoint[p.Name] {
				seenPoint[p.Name] = true
				cfg.CatalogTypes.HookTypes = append(cfg.CatalogTypes.HookTypes, p)
			}
		}
		policies, err := storage.LoadPoliciesLayered(fsys)
		if err != nil {
			t.Fatalf("load policies %s: %v", dir, err)
		}
		for _, p := range policies {
			if !seenPolicy[p.Name] {
				seenPolicy[p.Name] = true
				cfg.CatalogTypes.Policies = append(cfg.CatalogTypes.Policies, p)
			}
		}
	}
	return cfg, up
}

// behaviour is what one object answers.
type behaviour struct {
	typ           string
	level         bool     // a module or a project: the containment questions are asked under it
	caps, hooks   []string // in force, sorted
	writes, picks []string // the types a write accepts under it; the types « + New » offers
}

// behaviourOf reads every object of the corpus at root.
func behaviourOf(t *testing.T, base *CWMServiceConfig, root string) map[string]behaviour {
	t.Helper()
	c := *base
	c.WorkspaceRoot = freshWorkspace(t)
	c.ProjectRoot = root
	ctx := stdctx.Background()
	svc, err := New(ctx, &c)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	all, err := svc.ObjectList(ctx, api.ObjectListInput{})
	if err != nil {
		t.Fatal(err)
	}
	types := svc.objectTypes()
	out := map[string]behaviour{}
	for _, o := range all.Objects {
		b := behaviour{typ: o.Type}
		caps, err := svc.ObjectEffectiveCapabilities(ctx, api.ObjectEffectiveCapabilitiesInput{ID: o.ID})
		if err != nil {
			t.Fatalf("capabilities of %s: %v", o.ID, err)
		}
		for _, cr := range caps.Capabilities {
			b.caps = append(b.caps, cr.ID)
			b.level = b.level || cr.ID == "module" || cr.ID == "project"
		}
		hooks, err := svc.ObjectHooks(ctx, api.ObjectHooksInput{ID: o.ID})
		if err != nil {
			t.Fatalf("hooks of %s: %v", o.ID, err)
		}
		for _, h := range hooks.Hooks {
			b.hooks = append(b.hooks, string(h.Point)+" "+h.URI+" "+h.When)
		}
		if b.level {
			for name, ct := range types {
				if !ct.Abstract && svc.checkObjectParentChild(api.Object{ID: o.ID + ".probe", Type: name, Parent: o.ID}, "create") == nil {
					b.writes = append(b.writes, name)
				}
			}
			kids, err := svc.ObjectAllowedChildren(ctx, api.ObjectAllowedChildrenInput{ParentID: o.ID})
			if err != nil {
				t.Fatalf("children of %s: %v", o.ID, err)
			}
			for _, ch := range kids.Choices {
				b.picks = append(b.picks, ch.Name)
			}
		}
		for _, l := range [][]string{b.caps, b.hooks, b.writes, b.picks} {
			sort.Strings(l)
		}
		b.caps, b.hooks = slices.Compact(b.caps), slices.Compact(b.hooks)
		out[o.ID] = b
	}
	return out
}

// levelTypes are the bare level types a converted copy no longer holds, and
// the types they became: a write or a pick of one stands for the other.
var levelTypes = map[string]bool{"module": true, "project": true, "software-module": true, "documentation": true, "software-project": true}

// withoutLevelTypes drops the level types and the exempted ones from a list.
func withoutLevelTypes(l []string) []string {
	return slices.DeleteFunc(slices.Clone(l), func(n string) bool { return levelTypes[n] || typeNameAcceptParents[n] != "" })
}

// typeNamed is the declared type of that name.
func typeNamed(cfg *CWMServiceConfig, name string) (api.ObjectType, bool) {
	i := slices.IndexFunc(cfg.CatalogTypes.Objects, func(ot api.ObjectType) bool { return ot.Name == name })
	if i < 0 {
		return api.ObjectType{}, false
	}
	return cfg.CatalogTypes.Objects[i], true
}

// setDiff answers what of was is not in now, and what of now was not in was.
func setDiff(was, now []string) (lost, gained []string) {
	for _, n := range was {
		if !slices.Contains(now, n) {
			lost = append(lost, n)
		}
	}
	for _, n := range now {
		if !slices.Contains(was, n) {
			gained = append(gained, n)
		}
	}
	return lost, gained
}

func TestPlanningBehavesTheSameOnAConvertedCopy(t *testing.T) {
	long.Test(t)
	// The real-corpus tests wait mostly on the disk: run side by side, they
	// keep the package well within go test's default timeout under a full
	// go test./....
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	cfg, up := behaviourConfig(t)
	for _, r := range convertedRepositories {
		t.Run(r.name, func(t *testing.T) {
			repo := filepath.Join(up, r.dir)
			if err := exec.Command("git", "-C", repo, "cat-file", "-e", r.commit+"^{commit}").Run(); err != nil {
				t.Skipf("%s at %s is not here (a shallow or missing clone): %v", r.name, r.commit, err)
			}
			// The corpus out of git is the test's own copy: read as it stood,
			// then migrated where it stands and read again (no second copy).
			root := corpusAtTag(t, repo, r.commit)
			before := behaviourOf(t, cfg, root)
			migrateCopy(t, root, migrationSteps)
			after := behaviourOf(t, cfg, root)
			converted, levels := 0, 0
			for id, b := range before {
				a, ok := after[id]
				if !ok {
					t.Errorf("%s: gone after the conversion", id)
					continue
				}
				if a.typ != b.typ {
					converted++
				}
				if !slices.Equal(a.caps, withoutCodeOfDocumentation(a, b.caps)) && knownCapabilityChanges[id] == "" {
					t.Errorf("%s (%s -> %s): capabilities %v, before %v", id, b.typ, a.typ, a.caps, b.caps)
				}
				if !slices.Equal(a.hooks, b.hooks) {
					t.Errorf("%s (%s -> %s): hooks %v, before %v", id, b.typ, a.typ, a.hooks, b.hooks)
				}
				if !b.level {
					continue
				}
				levels++
				lost, gained := setDiff(withoutLevelTypes(b.writes), withoutLevelTypes(a.writes))
				// A type that names its own parents and never named the old
				// one was accepted only because a bare module curated nothing
				// (a change under a release, a compose-task under a
				// composition): a type that curates its children refuses it.
				lost = slices.DeleteFunc(lost, func(n string) bool {
					ct, ok := typeNamed(cfg, n)
					bt, declared := typeNamed(cfg, b.typ)
					if !declared {
						bt = api.ObjectType{Name: b.typ}
					}
					return ok && len(ct.AcceptParents) > 0 && !ct.IsParentAllowed(bt)
				})
				if len(lost)+len(gained) > 0 {
					t.Errorf("%s (%s -> %s): a write under it no longer accepts %v, now accepts %v", id, b.typ, a.typ, lost, gained)
				}
				// A type that curates its children (documentation) offers its
				// list; any other offers what « + New » offered before.
				if at, ok := typeNamed(cfg, a.typ); ok && a.typ != b.typ && len(at.AllowedChildren) > 0 {
					for _, name := range at.AllowedChildren {
						if !slices.Contains(a.picks, name) {
							t.Errorf("%s (%s): « + New » does not offer %s, a child its type curates", id, a.typ, name)
						}
					}
				} else if !slices.Equal(withoutLevelTypes(a.picks), withoutLevelTypes(b.picks)) {
					t.Errorf("%s (%s -> %s): « + New » offers %v, before %v", id, b.typ, a.typ, withoutLevelTypes(a.picks), withoutLevelTypes(b.picks))
				}
			}
			t.Logf("%s at %s: %d objects, %d converted, %d modules and projects compared", r.name, r.commit, len(before), converted, levels)
			if converted == 0 || levels == 0 {
				t.Fatalf("%s: %d converted, %d levels - nothing proven", r.name, converted, levels)
			}
		})
	}
}
