// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/appcore/selfupdate"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/authoring"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/index"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/keysfile"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/publish"
)

// publishedShape is what a marketplace build writes at the root of its output
// (the key's public half is added with its prefix): a reinit removes exactly
// this, anything else in the folder is left alone.
var publishedShape = []string{"katyoti-marketplace.yaml", "katyoti-marketplace.yaml.minisig",
	"sidecars", "config-sets", "seeds", "vaults", "solutions", "infra"}

// BuildFromDefinition publishes a whole marketplace from its definition: every
// check first — the key, the build's platforms, every source, every vault's
// password, every case — and nothing written while one fails; then the
// sidecars, the infra channel (naming the demo), the config-sets, the seeds,
// the vaults (sealed from their .env), the solutions (their cases refreshed
// from the build), the final gate, the mirrors.
func (s *Service) BuildFromDefinition(ctx context.Context, in api.BuildFromDefinitionInput) (out api.BuildFromDefinitionOutput, err error) {
	def, err := authoring.LoadDefinition(wsPath(in.Definition))
	if err != nil {
		return out, err
	}
	version := strings.TrimSpace(in.Version)
	if version == "" {
		return out, fmt.Errorf("--version is required")
	}
	dist := in.Dist
	if dist == "" {
		dist = "dist"
	}
	dist = wsPath(dist)
	whatsNew := ""
	if in.WhatsNew != "" {
		whatsNew = wsPath(in.WhatsNew)
	}
	out.Output, out.Version = def.Output, version
	channel := strings.TrimSpace(in.Channel)
	if channel != "" && channel != mktapi.ChannelStable && channel != mktapi.ChannelCanary {
		return out, fmt.Errorf("--channel %q: stable or canary", channel)
	}
	// A canary cut publishes with no retention: a version is stable until it
	// is marked canary, and would push a stable release out. Retention runs
	// once it is marked.
	keep := def.Keep
	if channel == mktapi.ChannelCanary {
		def.Keep = 0
	}
	if len(in.Platforms) > 0 {
		for _, p := range in.Platforms {
			if !slices.Contains(def.Platforms, p) {
				return out, fmt.Errorf("%s is not one of the definition's platforms (%s)", p, strings.Join(def.Platforms, ", "))
			}
		}
		def.Platforms = in.Platforms
	}
	say := func(format string, a ...any) { fmt.Fprintf(os.Stderr, "==> "+format+"\n", a...) }

	vaults, err := checkDefinition(def, dist, whatsNew, in.DryRun && in.BeforeBuild, &out)
	if err != nil {
		return out, err
	}
	// A cut publishes a new version, above the last one: never one already
	// published, never a lower one. Starting over (--reinit) wipes the tree,
	// so nothing published is compared.
	if !in.Reinit {
		last, err := authoring.CheckCutVersion(def.Output, version)
		if err != nil {
			return out, fmt.Errorf("nothing published: %w", err)
		}
		out.LastPublished = last
		if last != "" {
			say("version %s, above %s (the last published at %s)", version, last, def.Output)
		} else {
			say("version %s, the first published at %s", version, def.Output)
		}
	}
	if in.DryRun {
		out.Published = planned(def, version)
		return out, nil
	}

	// From here the tree is written: a failure (one the checks cannot foresee —
	// a disk, a share) says what is left and how to go on.
	defer func() {
		if err != nil {
			err = fmt.Errorf("%w\n%s is partly published at %s: publish again with a new version, or start over (--reinit)", err, def.Output, version)
		}
	}()
	secret, catalog := def.SecretKey(), filepath.Join(def.Output, "katyoti-marketplace.yaml")
	a := authoring.New()
	if in.Reinit {
		say("wiping the published tree at %s", def.Output)
		for _, p := range append(publishedShape, def.Key.Prefix+".key.pub") {
			if err := os.RemoveAll(filepath.Join(def.Output, p)); err != nil {
				return out, err
			}
		}
	}
	if err := os.MkdirAll(def.Output, 0o755); err != nil {
		return out, err
	}

	say("sidecars (%s)", strings.Join(def.Platforms, ", "))
	if _, err := a.Release(dist, def.Name, def.BaseURL, def.Output, def.Keep); err != nil {
		return out, fmt.Errorf("sidecars: %w", err)
	}
	if _, err := a.Sign(catalog, secret, ""); err != nil {
		return out, fmt.Errorf("sign: %w", err)
	}
	out.Published = append(out.Published, "sidecars@"+version)

	say("infra channel")
	demo := ""
	if def.Demo != "" {
		demo = def.Demo + "@" + version
	}
	if err := publishInfra(a, channel, dist, def.Output, version, secret, demo); err != nil {
		return out, fmt.Errorf("infra: %w", err)
	}
	out.Published = append(out.Published, "infra@"+version)

	for _, c := range def.ConfigSets {
		say("config-set %s", c.ItemName())
		display, desc := itemTexts(c.Path, c.DisplayName, c.Description)
		if _, err := a.BuildConfigSet(c.Path, c.ItemName(), version, display, desc, def.Output, def.Keep, secret); err != nil {
			return out, fmt.Errorf("config-set %s: %w", c.ItemName(), err)
		}
		out.Published = append(out.Published, "config-set "+c.ItemName()+"@"+version)
	}
	for _, sd := range def.Seeds {
		say("seed %s", sd.ItemName())
		display, desc := itemTexts(sd.Path, sd.DisplayName, sd.Description)
		if _, err := a.BuildSeed(sd.Path, sd.Tasks, sd.ItemName(), version, display, desc, nil, def.Output, def.Keep, secret, sd.Demo); err != nil {
			return out, fmt.Errorf("seed %s: %w", sd.ItemName(), err)
		}
		out.Published = append(out.Published, "seed "+sd.ItemName()+"@"+version)
	}
	for _, v := range def.Vaults {
		say("vault %s", v.Name)
		file, err := keysfile.Seal(vaults[v.Name], os.Getenv(v.PasswordEnv))
		if err != nil {
			return out, fmt.Errorf("vault %s: %w", v.Name, err)
		}
		tmp, err := os.CreateTemp("", "vault-*.sealed")
		if err != nil {
			return out, err
		}
		_, werr := tmp.Write(file)
		_ = tmp.Close()
		if werr == nil {
			_, werr = a.BuildVault(tmp.Name(), v.Name, version, def.Output, def.Keep, secret)
		}
		_ = os.Remove(tmp.Name())
		if werr != nil {
			return out, fmt.Errorf("vault %s: %w", v.Name, werr)
		}
		out.Published = append(out.Published, "vault "+v.Name+"@"+version)
	}
	for _, sol := range def.Solutions {
		say("solution %s (case %s)", sol.ItemName(), sol.Case)
		at := func(names []string) []string {
			refs := make([]string, len(names))
			for i, n := range names {
				refs[i] = n + "@" + version
			}
			return refs
		}
		var others map[string][]string
		for _, o := range sol.OtherUniverses {
			if others == nil {
				others = map[string][]string{}
			}
			others[o.Name] = at(o.ConfigSets)
		}
		covers := letterCovers(def.Dir(), sol, whatsNew, in.Covers)
		if _, err := publish.Solution(ctx, publish.SolutionInput{
			Case: sol.Case, Root: def.ScenarioRoot, Bin: filepath.Join(dist, hostPlatform()),
			Name: sol.ItemName(), Checkpoint: sol.Checkpoint, Version: version, Catalog: catalog, SecretKey: secret,
			ConfigSets: at(sol.ConfigSets), Vaults: at(sol.Vaults), Docs: sol.Docs, WhatsNew: whatsNew,
			LetterCovers: covers, LetterRequests: letterRequests(ctx, covers, changesOf(ctx), os.Stderr), OtherConfigSets: others, Out: os.Stderr,
		}); err != nil {
			return out, fmt.Errorf("solution %s: %w", sol.ItemName(), err)
		}
		out.Published = append(out.Published, "solution "+sol.ItemName()+"@"+version)
	}

	if channel == mktapi.ChannelCanary {
		// A release candidate: on canary, seen only by the alpha users.
		say("on canary")
		if _, err := a.MarkChannel(def.Output, version, mktapi.ChannelCanary, secret); err != nil {
			return out, fmt.Errorf("canary: %w", err)
		}
		if _, err := a.Prune(def.Output, keep, secret); err != nil {
			return out, fmt.Errorf("retention: %w", err)
		}
		out.Published = append(out.Published, "on canary")
	}

	// The tree holds only what is published: a swap's leftover left in it
	// (katyoti.exe.old-…) is removed before the gate, which
	// refuses one.
	swept, err := authoring.SweepLeftovers(def.Output)
	if err != nil {
		return out, fmt.Errorf("remove the leftovers of %s: %w", def.Output, err)
	}
	for _, rel := range swept {
		say("removed %s: a leftover, not a published item", rel)
	}
	say("final gate")
	sum, err := a.Publish(def.Output, def.PublicKey())
	if err != nil {
		return out, fmt.Errorf("the published tree does not verify: %w", err)
	}
	out.Summary = sum
	for _, m := range def.Mirrors {
		if filepath.Clean(m) == filepath.Clean(def.Output) {
			continue
		}
		say("mirror %s", m)
		if err := mirror(def.Output, m, def.Key.Prefix); err != nil {
			return out, fmt.Errorf("mirror %s: %w", m, err)
		}
		out.Mirrors = append(out.Mirrors, m)
	}
	return out, nil
}

// checkDefinition runs every check a build needs before it writes anything,

// publishInfra publishes the control plane of a cut. A candidate (canary)
// publishes it without moving the pointer every installation follows, stable
// ones included; its promotion points it. A stable cut
// publishes it and points the channel at it.
func publishInfra(a *authoring.Service, channel, dist, output, version, secret, demo string) error {
	build := a.BuildInfraChannel
	if channel == mktapi.ChannelCanary {
		build = a.BuildInfraVersion
	}
	_, err := build(dist, output, version, secret, demo)
	return err
}

// and answers each vault's keys (read from its .env).
func checkDefinition(def *authoring.Definition, dist, whatsNew string, noBuild bool, out *api.BuildFromDefinitionOutput) (map[string][]keysfile.Entry, error) {
	var problems []string
	bad := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }
	if _, err := os.Stat(def.SecretKey()); err != nil {
		bad("no signing key at %s (ktw-compose keygen --folder %s --prefix %s makes one; never commit it)", def.SecretKey(), def.Key.Folder, def.Key.Prefix)
	}
	built := map[string]bool{}
	distFound := false
	if noBuild {
		// Checked before the build exists: nothing of dist is asked.
	} else if entries, err := os.ReadDir(dist); err != nil {
		bad("no build at %s (make dist-all)", dist)
	} else {
		distFound = true
		for _, e := range entries {
			if e.IsDir() && strings.Contains(e.Name(), "-") {
				built[e.Name()] = true
			}
		}
	}
	for _, p := range def.Platforms {
		if !built[p] && !noBuild {
			bad("the build at %s has no %s", dist, p)
		} else if !noBuild {
			// The infra channel is published from the same build: a platform
			// without its control plane would fail after the catalog is written.
			exe := "katyoti"
			if strings.HasPrefix(p, "windows-") {
				exe += ".exe"
			}
			if _, err := os.Stat(filepath.Join(dist, p, exe)); err != nil {
				bad("the build's %s has no control plane (%s)", p, exe)
			}
		}
		delete(built, p)
	}
	for p := range built {
		bad("the build at %s holds %s, which the definition does not publish (rebuild dist with its platforms only)", dist, p)
	}
	if distFound {
		if err := index.VerifyBuild(dist); err != nil {
			bad("%v", err)
		} else if platforms, err := index.DiscoverPlatforms(dist); err != nil {
			bad("%v", err)
		} else if built, err := index.Build(platforms, def.Name, def.BaseURL, index.LayoutVersioned); err != nil {
			bad("index the build: %v", err)
		} else {
			// Every sidecar of the build is indexed before anything is
			// written: one that cannot describe itself would be missing from
			// the catalog, and a solution pinning it would fail half-published.
			for _, sk := range built.Skips {
				if sk.Failed {
					bad("%s cannot be published: %s", sk.Binary, sk.Reason)
				}
			}
		}
	}
	if len(def.Solutions) > 0 && !noBuild {
		if _, err := os.Stat(filepath.Join(dist, hostPlatform())); err != nil {
			bad("the solutions run this machine's build (%s), which %s does not hold", hostPlatform(), dist)
		}
	}
	for _, c := range def.ConfigSets {
		if _, err := os.Stat(c.Path); err != nil {
			bad("config-set %s: no folder %s", c.ItemName(), c.Path)
		}
	}
	for _, sd := range def.Seeds {
		if _, err := os.Stat(sd.Path); err != nil {
			bad("seed %s: no folder %s", sd.ItemName(), sd.Path)
		}
	}
	vaults := map[string][]keysfile.Entry{}
	for _, v := range def.Vaults {
		if _, err := os.Stat(v.EnvFile); err != nil {
			bad("vault %s: no %s", v.Name, v.EnvFile)
			continue
		}
		if os.Getenv(v.PasswordEnv) == "" {
			bad("vault %s: its password is read from %s, which is not set", v.Name, v.PasswordEnv)
		}
		keys, missing, err := keysfile.FromEnvFile(v.EnvFile, v.Keys)
		switch {
		case err != nil:
			bad("vault %s: %v", v.Name, err)
		case len(keys) == 0:
			bad("vault %s: %s has a value for none of the keys %s declares", v.Name, v.EnvFile, v.Keys)
		default:
			vaults[v.Name] = keys
			if len(missing) > 0 {
				out.Warnings = append(out.Warnings, fmt.Sprintf("vault %s: no value for %s (left out)", v.Name, strings.Join(missing, ", ")))
			}
		}
	}
	for _, sol := range def.Solutions {
		if err := publish.Check(sol.Case); err != nil {
			bad("solution %s: %v", sol.ItemName(), err)
		}
	}
	if whatsNew != "" {
		if _, err := os.Stat(whatsNew); err != nil {
			bad("no change log at %s", whatsNew)
		}
	}
	problems = append(problems, scanArtefacts(def, dist, whatsNew, noBuild)...)
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, fmt.Errorf("nothing published:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return vaults, nil
}

// planned is what a build would publish.
func planned(def *authoring.Definition, version string) []string {
	p := []string{"sidecars@" + version + " (" + strings.Join(def.Platforms, ", ") + ")", "infra@" + version}
	for _, c := range def.ConfigSets {
		p = append(p, "config-set "+c.ItemName()+"@"+version)
	}
	for _, sd := range def.Seeds {
		p = append(p, "seed "+sd.ItemName()+"@"+version)
	}
	for _, v := range def.Vaults {
		p = append(p, "vault "+v.Name+"@"+version)
	}
	for _, sol := range def.Solutions {
		p = append(p, "solution "+sol.ItemName()+"@"+version)
	}
	return p
}

// itemTexts are an item's display name and description: given, else the first
// line of display-name.txt / description.txt in its folder.
func itemTexts(folder, display, desc string) (string, string) {
	first := func(name string) string {
		b, err := os.ReadFile(filepath.Join(folder, name))
		if err != nil {
			return ""
		}
		line, _, _ := strings.Cut(string(b), "\n")
		return strings.TrimSpace(line)
	}
	if display == "" {
		display = first("display-name.txt")
	}
	if desc == "" {
		desc = first("description.txt")
	}
	return display, desc
}

func hostPlatform() string { return runtime.GOOS + "-" + runtime.GOARCH }

// mirror makes dst hold the published tree: its published shape replaced by
// src's, a swap's leftover removed wherever it is, anything else in dst left
// alone.
func mirror(src, dst, keyPrefix string) error {
	for _, p := range append(publishedShape, keyPrefix+".key.pub") {
		if err := os.RemoveAll(filepath.Join(dst, p)); err != nil {
			return err
		}
	}
	if _, err := authoring.SweepLeftovers(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if selfupdate.IsLeftover(d.Name()) {
			return nil // never copied: not a published item
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		o, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(o, in); err != nil {
			_ = o.Close()
			return err
		}
		return o.Close()
	})
}

// letterCovers are the change logs a solution's written « Quoi de neuf »
// covers, so the build checks its figures against all of them
// : the build's --whats-new, the --covers it
// is given and the solution's letter_covers (relative to the definition's
// folder), each once; empty when the letter covers --whats-new alone.
func letterCovers(defDir string, sol authoring.DefinitionSolution, whatsNew string, covers []string) []string {
	var out []string
	add := func(p string) {
		if p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	add(whatsNew)
	for _, c := range covers {
		add(wsPath(c))
	}
	for _, c := range sol.LetterCovers {
		if !filepath.IsAbs(c) {
			c = filepath.Join(defDir, c)
		}
		add(c)
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// changeLister lists a release's change entries: their requests.
type changeLister func(release string) ([]planapi.Change, error)

// changesOf lists change entries through planification's changes verb
// (ktw-agents:changes/list), run as any other sidecar's method; nil when no
// action service answers (the build then leaves the requests figure
// unchecked).
func changesOf(ctx context.Context) changeLister {
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil || actions == nil {
		return nil
	}
	return func(release string) ([]planapi.Change, error) {
		in, _ := json.Marshal(planapi.ListChangesInput{Release: release})
		out, err := actions.Run(ctx, actionsapi.RunInput{URI: "ktw-agents:changes/list", Input: in})
		if err != nil {
			return nil, err
		}
		var got planapi.ListChangesOutput
		if err := json.Unmarshal(out.Value, &got); err != nil {
			return nil, err
		}
		return got.Changes, nil
	}
}

// letterRequests is how many distinct requests the changes of the change
// logs a written « Quoi de neuf » covers name, so the build
// checks the letter's requests figure: each change log is a release's page,
// its file named by the release's version (2026.10.md, 1.006.md). 0 - not
// checked - when the changes cannot be listed, said on out.
func letterRequests(ctx context.Context, covers []string, list changeLister, out io.Writer) int {
	if len(covers) == 0 {
		return 0
	}
	if list == nil {
		fmt.Fprintln(out, "the letter's requests figure is not checked: no changes service answers")
		return 0
	}
	seen := map[string]bool{}
	for _, c := range covers {
		version := strings.TrimSuffix(filepath.Base(c), filepath.Ext(c))
		changes, err := list(version)
		if err != nil {
			fmt.Fprintf(out, "the letter's requests figure is not checked: the changes of %s: %v\n", version, err)
			return 0
		}
		for _, ch := range changes {
			for _, r := range ch.Requests {
				seen[r] = true
			}
		}
	}
	return len(seen)
}
