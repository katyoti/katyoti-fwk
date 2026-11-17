// SPDX-License-Identifier: Apache-2.0

//go:build katyoti_e2e

// Package e2etest is the katyoti end-to-end regression harness. It builds the
// katyoti (consumer) + ktw-compose (publisher) binaries once, then drives them
// through the flows that were verified live across ms-katyoti — so a regression
// in the marketplace / trust / content / release / authoring surface is caught
// automatically. CLI-only: no hub, no GUI (those paths are covered by package
// unit tests + a manual GUI smoke).
//
// Run:  go test -tags katyoti_e2e ./internal/extensions/marketplace/e2etest/...
//
//	or: make katyoti-e2e
package e2etest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	katyotiBin string
	composeBin string
	hostBin    string
)

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "katyoti-e2e-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mktemp:", err)
		os.Exit(1)
	}
	katyotiBin = filepath.Join(dir, "katyoti"+exeSuffix())
	composeBin = filepath.Join(dir, "ktw-compose"+exeSuffix())
	hostBin = filepath.Join(dir, "ktw-host"+exeSuffix())
	for _, b := range []struct{ out, pkg string }{
		{katyotiBin, "github.com/katyoti/katyoti-fwk/app/katyoti"},
		{composeBin, "github.com/katyoti/katyoti-fwk/app/ktw-compose"},
		{hostBin, "github.com/katyoti/katyoti-fwk/app/ktw-host"},
	} {
		cmd := exec.Command("go", "build", "-o", b.out, b.pkg)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "build", b.pkg, ":", err)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// --- isolated instance (its own global root + workspace + caches) ------------

type inst struct {
	root      string // KTW_KATYOTI_ROOT (global: store, overlay)
	workspace string // KTW_WORKSPACE
	cache     string
	cfg       string
	// home, when set, is the person's home its programs see (USERPROFILE,
	// HOME): what they put in the person's documents - a project in the
	// projects folder - stays in the test's world.
	home string
}

// withHome is the instance with a home of its own.
func (in inst) withHome(home string) inst {
	in.home = home
	return in
}

func newInst(t *testing.T) inst {
	t.Helper()
	base := t.TempDir()
	in := inst{
		root:      filepath.Join(base, "root", "katyoti"),
		workspace: filepath.Join(base, "ws"),
		cache:     filepath.Join(base, "cache"),
		cfg:       filepath.Join(base, "cfg"),
	}
	for _, d := range []string{in.root, filepath.Join(in.workspace, ".katyoti", "config"), in.cache, in.cfg} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return in
}

// env is the instance's environment. KTW_NO_INSTALLATION keeps the programs
// it starts on its own root even when the test runs from a program of an
// installation (an agent's check run by a hub): they would otherwise take the
// inherited installation back and work on its registries and marketplaces.
func (in inst) env() []string {
	env := append(os.Environ(),
		"KTW_NO_INSTALLATION=1",
		"KTW_KATYOTI_ROOT="+in.root,
		"KTW_WORKSPACE="+in.workspace,
		"XDG_CACHE_HOME="+in.cache,
		"XDG_CONFIG_HOME="+in.cfg,
	)
	if in.home != "" {
		env = append(env, "USERPROFILE="+in.home, "HOME="+in.home)
	}
	return env
}

// run executes a binary in this instance; returns combined output. Fails the
// test on a non-zero exit unless wantErr.
func (in inst) run(t *testing.T, wantErr bool, bin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = in.env()
	out, err := cmd.CombinedOutput()
	if wantErr && err == nil {
		t.Fatalf("%s %v: expected an error, got success\n%s", filepath.Base(bin), args, out)
	}
	if !wantErr && err != nil {
		t.Fatalf("%s %v: %v\n%s", filepath.Base(bin), args, err, out)
	}
	return string(out)
}

// jsonField pulls a top-level string/number field out of an auto-CLI JSON reply.
func jsonField(t *testing.T, out, field string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("parse json (%s): %v\n%s", field, err, out)
	}
	switch v := m[field].(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// contentPack writes a files-only sidecar folder + tar.gz's it; returns the
// archive path + its sha256.
func contentPack(t *testing.T, dir, name string) (archive, sha string) {
	t.Helper()
	pack := filepath.Join(dir, "pack")
	writeFile(t, filepath.Join(pack, "ktw-host.yaml"), "name: "+name+"\ndescription: e2e content sidecar\n")
	writeFile(t, filepath.Join(pack, "skills", "demo.md"), "---\nname: demo\ndescription: A demo skill.\n---\nbody\n")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = filepath.Walk(pack, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(pack, p)
		data, _ := os.ReadFile(p)
		_ = tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg})
		_, _ = tw.Write(data)
		return nil
	})
	_ = tw.Close()
	_ = gz.Close()

	archive = filepath.Join(dir, name+"-1.0.0.tar.gz")
	if err := os.WriteFile(archive, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return archive, hex.EncodeToString(sum[:])
}

func fileURL(p string) string { return "file://" + filepath.ToSlash(p) }

// --- publisher chain: keygen → build → sign → publish (verify) ---------------

func TestPublisherChain(t *testing.T) {
	in := newInst(t)
	// a source folder with one real binary to describe
	src := filepath.Join(in.workspace, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFile(t, hostBin, filepath.Join(src, "ktw-host"+exeSuffix()))

	// keygen (folder + prefix) → keys land in the folder
	out := in.run(t, false, composeBin, "keygen", "--folder", "keys", "--prefix", "team")
	sec := jsonField(t, out, "secretKeyPath")
	if !strings.HasSuffix(sec, "team.key") {
		t.Fatalf("secret key path = %q", sec)
	}
	if _, err := os.Stat(filepath.Join(in.workspace, "keys", "team.key.pub")); err != nil {
		t.Fatalf("pubkey not written under the workspace: %v", err)
	}

	// build (relative source → workspace) → catalog
	out = in.run(t, false, composeBin, "build-marketplace", "src", "--name", "demo", "--output", "out/katyoti-marketplace.yaml")
	if n := jsonField(t, out, "itemCount"); n != "1" {
		t.Fatalf("itemCount = %q, want 1\n%s", n, out)
	}
	catalog := filepath.Join(in.workspace, "out", "katyoti-marketplace.yaml")
	if _, err := os.Stat(catalog); err != nil {
		t.Fatalf("catalog not created under the workspace: %v", err)
	}

	// sign (folder + prefix only)
	out = in.run(t, false, composeBin, "sign-marketplace", "out/katyoti-marketplace.yaml", "--key-folder", "keys", "--key-prefix", "team")
	if sig := jsonField(t, out, "signaturePath"); !strings.HasSuffix(sig, ".minisig") {
		t.Fatalf("signaturePath = %q", sig)
	}

	// publish → verifies the signature
	out = in.run(t, false, composeBin, "publish-marketplace", "out/katyoti-marketplace.yaml", "--key-folder", "keys", "--key-prefix", "team")
	if s := jsonField(t, out, "summary"); !strings.Contains(s, "VERIFIED") {
		t.Fatalf("publish summary missing VERIFIED: %q", s)
	}

	// tamper the catalog → publish must refuse (do-not-distribute gate)
	appendFile(t, catalog, "\n# tampered\n")
	out = in.run(t, true, composeBin, "publish-marketplace", "out/katyoti-marketplace.yaml", "--key-folder", "keys", "--key-prefix", "team")
	if !strings.Contains(out, "does NOT verify") {
		t.Fatalf("tampered publish should refuse; got:\n%s", out)
	}
}

// TestSignTrailingSpacePath: a stray trailing space in the output path (from a
// config form) must NOT corrupt the signature filename — Windows swallows a
// trailing space on open, so `path + ".minisig"` would otherwise become
// `…yaml .minisig` that the consumer (looking for `…yaml.minisig`) can't find.
func TestSignTrailingSpacePath(t *testing.T) {
	in := newInst(t)
	src := filepath.Join(in.workspace, "src")
	_ = os.MkdirAll(src, 0o755)
	copyFile(t, hostBin, filepath.Join(src, "ktw-host"+exeSuffix()))
	in.run(t, false, composeBin, "keygen", "--folder", "keys", "--prefix", "team")
	in.run(t, false, composeBin, "build-marketplace", "src", "--name", "demo", "--output", "out/katyoti-marketplace.yaml ")
	in.run(t, false, composeBin, "sign-marketplace", "out/katyoti-marketplace.yaml ", "--key-folder", "keys", "--key-prefix", "team")
	// the signature must be at the clean path, and there must be NO spaced variant
	if _, err := os.Stat(filepath.Join(in.workspace, "out", "katyoti-marketplace.yaml.minisig")); err != nil {
		t.Fatalf("signature not at the clean path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(in.workspace, "out", "katyoti-marketplace.yaml .minisig")); err == nil {
		t.Fatal("a space-corrupted .minisig was written")
	}
}

// TestRelativeAssetURLPortableFolder: build with DEFAULT (relative) asset URLs,
// then COPY the whole folder elsewhere and install from the copy — the relative
// asset resolves against the (moved) catalog's location, so the folder is
// portable (no rewrite / re-sign needed).
func TestRelativeAssetURLPortableFolder(t *testing.T) {
	pub := newInst(t)
	src := filepath.Join(pub.workspace, "market")
	_ = os.MkdirAll(src, 0o755)
	copyFile(t, hostBin, filepath.Join(src, "ktw-host"+exeSuffix()))
	pub.run(t, false, composeBin, "build-marketplace", "market", "--name", "demo")

	// the catalog's asset URL must be RELATIVE (no absolute workspace path/scheme)
	cat, _ := os.ReadFile(filepath.Join(src, "katyoti-marketplace.yaml"))
	if strings.Contains(string(cat), filepath.ToSlash(pub.workspace)) || strings.Contains(string(cat), pub.workspace) {
		t.Fatalf("asset URL should be relative, not contain the abs workspace path:\n%s", cat)
	}

	// copy the whole folder to a fresh location (portability)
	moved := filepath.Join(t.TempDir(), "moved")
	copyDir(t, src, moved)

	// consumer installs from the MOVED catalog → relative asset resolves against it
	con := newInst(t)
	con.run(t, false, katyotiBin, "marketplace", "add", "demo", filepath.Join(moved, "katyoti-marketplace.yaml"))
	con.run(t, false, katyotiBin, "install", "ktw-host")
	if out := con.run(t, false, katyotiBin, "installed"); !strings.Contains(out, "ktw-host") {
		t.Fatalf("relative-URL install from the moved folder should land in the store:\n%s", out)
	}
}

// --- consumer trust: pinned add → verify → tamper refuse ---------------------

func TestConsumerSignedTrust(t *testing.T) {
	pub := newInst(t) // publisher builds+signs a marketplace
	src := filepath.Join(pub.workspace, "src")
	_ = os.MkdirAll(src, 0o755)
	copyFile(t, hostBin, filepath.Join(src, "ktw-host"+exeSuffix()))
	pub.run(t, false, composeBin, "keygen", "--folder", "keys", "--prefix", "team")
	pub.run(t, false, composeBin, "build-marketplace", "src", "--name", "team")
	pub.run(t, false, composeBin, "sign-marketplace", "src/katyoti-marketplace.yaml", "--key-folder", "keys", "--key-prefix", "team")
	catalog := filepath.Join(src, "katyoti-marketplace.yaml")
	pubkey := filepath.Join(pub.workspace, "keys", "team.key.pub")

	// consumer pins the key
	con := newInst(t)
	con.run(t, false, katyotiBin, "marketplace", "add", "team", fileURL(catalog), "--pubkey", pubkey, "--trust", "pinned")
	// show → verifies + lists
	if out := con.run(t, false, katyotiBin, "marketplace", "show", "team"); !strings.Contains(out, "ktw-host") {
		t.Fatalf("pinned show should list the item:\n%s", out)
	}
	// tamper → refuse
	appendFile(t, catalog, "\n# tampered\n")
	out := con.run(t, true, katyotiBin, "marketplace", "show", "team")
	if !strings.Contains(out, "did NOT verify") && !strings.Contains(out, "verify") {
		t.Fatalf("tampered pinned catalog should refuse:\n%s", out)
	}
}

// --- content install: pack → install → uninstall -----------------------------

func TestContentInstall(t *testing.T) {
	in := newInst(t)
	dist := filepath.Join(t.TempDir(), "dist")
	_ = os.MkdirAll(dist, 0o755)
	archive, sha := contentPack(t, dist, "flowpack")
	catalog := filepath.Join(dist, "katyoti-marketplace.yaml")
	writeFile(t, catalog, fmt.Sprintf(`apiVersion: 1
marketplace: {name: content}
items:
  - name: flowpack
    kind: content
    versions: [{version: "1.0.0", assets: [{url: "%s", sha256: "%s"}]}]
`, fileURL(archive), sha))

	in.run(t, false, katyotiBin, "marketplace", "add", "content", catalog)
	in.run(t, false, katyotiBin, "install", "flowpack")
	// The marketplace offers the content's runtime too: installing content
	// installs ktw-host with it, so wiring never takes one from PATH.
	hostData, err := os.ReadFile(hostBin)
	if err != nil {
		t.Fatal(err)
	}
	hostSum := sha256.Sum256(hostData)
	hostCatalog := filepath.Join(dist, "host.yaml")
	writeFile(t, hostCatalog, fmt.Sprintf(`apiVersion: 1
marketplace: {name: hosts}
items:
  - name: ktw-host
    kind: sidecar
    versions: [{version: "1.0.0", assets: [{os: %s, arch: %s, url: "%s", sha256: "%s"}]}]
  - name: flowpack2
    kind: content
    versions: [{version: "1.0.0", assets: [{url: "%s", sha256: "%s"}]}]
`, runtime.GOOS, runtime.GOARCH, fileURL(hostBin), hex.EncodeToString(hostSum[:]), fileURL(archive), sha))
	in.run(t, false, katyotiBin, "marketplace", "add", "hosts", hostCatalog)
	in.run(t, false, katyotiBin, "install", "flowpack2")
	if _, err := os.Stat(filepath.Join(in.root, "bin", "store", "ktw-host", "1.0.0", "ktw-host"+exeSuffix())); err != nil {
		out := in.run(t, false, katyotiBin, "installed")
		t.Fatalf("installing a content item should install its runtime ktw-host into the store (%v):\n%s", err, out)
	}
	// unpacked into the content store
	if _, err := os.Stat(filepath.Join(in.root, "bin", "store-content", "flowpack", "1.0.0", "ktw-host.yaml")); err != nil {
		t.Fatalf("content not unpacked into store-content: %v", err)
	}
	if out := in.run(t, false, katyotiBin, "installed"); !strings.Contains(out, "flowpack") || !strings.Contains(out, "content") {
		t.Fatalf("installed should show the content item:\n%s", out)
	}
	// bad checksum rejected
	writeFile(t, filepath.Join(dist, "bad.yaml"), fmt.Sprintf(`apiVersion: 1
marketplace: {name: bad}
items:
  - name: badpack
    kind: content
    versions: [{version: "1.0.0", assets: [{url: "%s", sha256: "deadbeef"}]}]
`, fileURL(archive)))
	in.run(t, false, katyotiBin, "marketplace", "add", "bad", filepath.Join(dist, "bad.yaml"))
	if out := in.run(t, true, katyotiBin, "install", "badpack"); !strings.Contains(out, "sha256 mismatch") {
		t.Fatalf("bad checksum should be rejected:\n%s", out)
	}
	// uninstall clears it
	in.run(t, false, katyotiBin, "uninstall", "flowpack")
	if _, err := os.Stat(filepath.Join(in.root, "bin", "store-content", "flowpack")); !os.IsNotExist(err) {
		t.Fatalf("uninstall should remove the content folder")
	}
}

// --- release: install + wire → snapshot → bootstrap into a fresh instance ----

func TestReleaseSnapshotBootstrap(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	_ = os.MkdirAll(dist, 0o755)
	archive, sha := contentPack(t, dist, "flowpack")
	contentCat := filepath.Join(dist, "katyoti-marketplace.yaml")
	writeFile(t, contentCat, fmt.Sprintf(`apiVersion: 1
marketplace: {name: content}
items:
  - name: flowpack
    kind: content
    versions: [{version: "1.0.0", assets: [{url: "%s", sha256: "%s"}]}]
`, fileURL(archive), sha))

	// SOURCE: install + wire into a universe
	src := newInst(t)
	writeFile(t, filepath.Join(src.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\ndisplay_name: Demo\n")
	src.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	src.run(t, false, katyotiBin, "install", "flowpack")
	src.run(t, false, katyotiBin, "universe", "wire", "flowpack")

	// snapshot the source universe → a release in a releases catalog
	releases := filepath.Join(dist, "releases.yaml")
	src.run(t, false, composeBin, "snapshot-universe", src.workspace, "--version", "v1", "--marketplace", releases, "--name", "demo-rel")
	// immutability: re-snapshot v1 refused
	src.run(t, true, composeBin, "snapshot-universe", src.workspace, "--version", "v1", "--marketplace", releases, "--name", "demo-rel")

	// TARGET: fresh instance subscribes to both, bootstraps the release
	tgt := newInst(t)
	writeFile(t, filepath.Join(tgt.workspace, ".katyoti", "config", "Univers.yaml"), "name: reproduced\n")
	tgt.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	tgt.run(t, false, katyotiBin, "marketplace", "add", "releases", releases)
	tgt.run(t, false, katyotiBin, "solution", "install", "demo-rel@v1", "--universe", tgt.workspace)
	// the member landed in the TARGET's own store-content
	if _, err := os.Stat(filepath.Join(tgt.root, "bin", "store-content", "flowpack", "1.0.0")); err != nil {
		t.Fatalf("bootstrap should install the pinned member into the target store: %v", err)
	}
	// the reproduced universe references the TARGET's local store path (not the source's)
	uni, _ := os.ReadFile(filepath.Join(tgt.workspace, ".katyoti", "config", "Univers.yaml"))
	if !strings.Contains(string(uni), "flowpack") {
		t.Fatalf("reproduced Univers.yaml missing the member:\n%s", uni)
	}
	if strings.Contains(string(uni), src.root) {
		t.Fatalf("reproduced wiring must NOT reference the source machine's store path:\n%s", uni)
	}
}

// TestFreezeToExe: freeze a content folder into a single exe, then run it with
// NO --folder — it self-extracts + serves the embedded folder under the
// manifest identity.
func TestFreezeToExe(t *testing.T) {
	in := newInst(t)
	pack := filepath.Join(in.workspace, "pack")
	writeFile(t, filepath.Join(pack, "ktw-host.yaml"), "name: northwind-flows\ndisplayName: NORTHWIND Flows\n")
	writeFile(t, filepath.Join(pack, "skills", "flow-spec.md"), "---\nname: flow-spec\ndescription: A flow.\n---\nbody")

	out := in.run(t, false, composeBin, "freeze-host", "pack", "--host", hostBin, "--output", "out/northwind-flows"+exeSuffix())
	exe := jsonField(t, out, "exePath")
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("frozen exe not created: %v", err)
	}

	// run the frozen exe with NO --folder → it adopts the embedded manifest
	cmd := exec.Command(exe, "describe", "--out", "json")
	cmd.Env = in.env()
	desc, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("frozen exe `describe`: %v\n%s", err, desc)
	}
	if !strings.Contains(string(desc), "northwind-flows") {
		t.Fatalf("frozen exe should self-identify as northwind-flows:\n%s", desc)
	}
}

// TestMultiMarketplaceNameCollision proves install is marketplace-QUALIFIED:
// when the same item name is served by two subscribed marketplaces, a bare
// install is refused as ambiguous (never silently first-matched), --marketplace
// resolves the right catalog, and a version-not-found error names the
// marketplace + the versions that ARE available. Regression guard for the live
// `gui-demo: version "…" not found` bug (install resolved the wrong catalog).
func TestMultiMarketplaceNameCollision(t *testing.T) {
	in := newInst(t)
	dist := filepath.Join(t.TempDir(), "dist")
	_ = os.MkdirAll(dist, 0o755)
	archive, sha := contentPack(t, dist, "dup")

	// two marketplaces, each serving "dup" at a DIFFERENT version
	alphaCat := filepath.Join(dist, "alpha.yaml")
	writeFile(t, alphaCat, fmt.Sprintf(`apiVersion: 1
marketplace: {name: alpha}
items:
  - name: dup
    kind: content
    versions: [{version: "1.0.0", assets: [{url: "%s", sha256: "%s"}]}]
`, fileURL(archive), sha))
	betaCat := filepath.Join(dist, "beta.yaml")
	writeFile(t, betaCat, fmt.Sprintf(`apiVersion: 1
marketplace: {name: beta}
items:
  - name: dup
    kind: content
    versions: [{version: "2.0.0", assets: [{url: "%s", sha256: "%s"}]}]
`, fileURL(archive), sha))

	in.run(t, false, katyotiBin, "marketplace", "add", "alpha", alphaCat)
	in.run(t, false, katyotiBin, "marketplace", "add", "beta", betaCat)

	// bare install → refused as ambiguous, naming both marketplaces
	out := in.run(t, true, katyotiBin, "install", "dup")
	if !strings.Contains(out, "served by 2 marketplaces") || !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("ambiguous bare install should be refused naming both marketplaces:\n%s", out)
	}

	// qualified install → resolves the chosen catalog (beta's 2.0.0)
	in.run(t, false, katyotiBin, "install", "dup", "--marketplace", "beta")
	if _, err := os.Stat(filepath.Join(in.root, "bin", "store-content", "dup", "2.0.0", "ktw-host.yaml")); err != nil {
		t.Fatalf("qualified install should land beta's 2.0.0 in the store: %v", err)
	}

	// wrong version in a chosen marketplace → error names the marketplace + available versions
	out = in.run(t, true, katyotiBin, "install", "dup@1.0.0", "--marketplace", "beta")
	if !strings.Contains(out, `"beta"`) || !strings.Contains(out, "available: 2.0.0") {
		t.Fatalf("version-not-found should name the marketplace + available versions:\n%s", out)
	}
}

// TestWireUnwireUninstallGuard proves the USED-state lifecycle: install → wire
// into a universe → uninstall is REFUSED while wired (would strand Univers.yaml)
// → unwire → uninstall now succeeds. Regression guard for the "can't remove /
// can't unwire" UX gap.
func TestWireUnwireUninstallGuard(t *testing.T) {
	in := newInst(t)
	writeFile(t, filepath.Join(in.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\ndisplay_name: Demo\n")

	dist := filepath.Join(t.TempDir(), "dist")
	_ = os.MkdirAll(dist, 0o755)
	archive, sha := contentPack(t, dist, "flowpack")
	catalog := filepath.Join(dist, "katyoti-marketplace.yaml")
	writeFile(t, catalog, fmt.Sprintf(`apiVersion: 1
marketplace: {name: content}
items:
  - name: flowpack
    kind: content
    versions: [{version: "1.0.0", assets: [{url: "%s", sha256: "%s"}]}]
`, fileURL(archive), sha))

	in.run(t, false, katyotiBin, "marketplace", "add", "content", catalog)
	in.run(t, false, katyotiBin, "install", "flowpack")
	in.run(t, false, katyotiBin, "universe", "wire", "flowpack")

	// wired → uninstall refused, pointing at unwire
	out := in.run(t, true, katyotiBin, "uninstall", "flowpack")
	if !strings.Contains(out, "unwire") {
		t.Fatalf("uninstall of a wired item should be refused and point to unwire:\n%s", out)
	}
	// the store is intact after the refused uninstall
	if _, err := os.Stat(filepath.Join(in.root, "bin", "store-content", "flowpack", "1.0.0")); err != nil {
		t.Fatalf("refused uninstall must not have removed the store: %v", err)
	}

	// unwire → then uninstall succeeds
	in.run(t, false, katyotiBin, "universe", "unwire", "flowpack")
	in.run(t, false, katyotiBin, "uninstall", "flowpack")
	if _, err := os.Stat(filepath.Join(in.root, "bin", "store-content", "flowpack")); !os.IsNotExist(err) {
		t.Fatalf("uninstall after unwire should remove the store folder")
	}
	// uninstalling something not installed is a clear error, not a silent no-op
	if out := in.run(t, true, katyotiBin, "uninstall", "flowpack"); !strings.Contains(out, "not installed") {
		t.Fatalf("uninstall of a missing item should error 'not installed':\n%s", out)
	}
}

// TestSetVersionUpdateDowngrade proves the version-picker engine: list versions
// newest-first, update to latest, downgrade to an older version WITHOUT re-fetch
// (both store dirs coexist), and repoint a wired universe pin on version change.
func TestSetVersionUpdateDowngrade(t *testing.T) {
	pub := newInst(t)
	market := filepath.Join(pub.workspace, "market")
	d1 := buildSidecarAt(t, filepath.Join(pub.workspace, "dist1"), "e2e-v1")
	pub.run(t, false, composeBin, "release-marketplace", d1, "--output", market, "--name", "demo", "--keep", "3")
	d2 := buildSidecarAt(t, filepath.Join(pub.workspace, "dist2"), "e2e-v2")
	pub.run(t, false, composeBin, "release-marketplace", d2, "--output", market, "--name", "demo", "--keep", "3")
	catalog := filepath.Join(market, "katyoti-marketplace.yaml")

	con := newInst(t)
	writeFile(t, filepath.Join(con.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\n")
	con.run(t, false, katyotiBin, "marketplace", "add", "demo", fileURL(catalog))

	// install the OLD version
	con.run(t, false, katyotiBin, "install", "ktw-host@e2e-v1")

	// versions → newest-first (e2e-v2 before e2e-v1, latest tag on the first)
	vs := con.run(t, false, katyotiBin, "versions", "ktw-host")
	if i2, i1 := strings.Index(vs, "e2e-v2"), strings.Index(vs, "e2e-v1"); i2 < 0 || i1 < 0 || i2 > i1 {
		t.Fatalf("versions should list e2e-v2 before e2e-v1 (newest first):\n%s", vs)
	}

	// wire it, so a version change must repoint the universe pin
	con.run(t, false, katyotiBin, "universe", "wire", "ktw-host")

	// update to latest → record + pin move to e2e-v2
	con.run(t, false, katyotiBin, "update", "ktw-host")
	uni, _ := os.ReadFile(filepath.Join(con.workspace, ".katyoti", "config", "Univers.yaml"))
	if !strings.Contains(filepath.ToSlash(string(uni)), "store/ktw-host/e2e-v2") {
		t.Fatalf("wired pin should repoint to e2e-v2 after update:\n%s", uni)
	}

	// downgrade to e2e-v1 (already in the store → no re-fetch); pin repoints back
	con.run(t, false, katyotiBin, "update", "ktw-host", "--to", "e2e-v1")
	uni, _ = os.ReadFile(filepath.Join(con.workspace, ".katyoti", "config", "Univers.yaml"))
	if !strings.Contains(filepath.ToSlash(string(uni)), "store/ktw-host/e2e-v1") {
		t.Fatalf("downgrade should repoint the pin to e2e-v1:\n%s", uni)
	}

	// both versions coexist in the store (nothing orphaned)
	for _, v := range []string{"e2e-v1", "e2e-v2"} {
		if _, err := os.Stat(filepath.Join(con.root, "bin", "store", "ktw-host", v, "ktw-host"+exeSuffix())); err != nil {
			t.Fatalf("expected %s to remain in the store after up/down: %v", v, err)
		}
	}
}

// TestSelfUpdateSwapsControlPlaneBinary proves the control-plane self-update path
// end-to-end WITHOUT a hosted channel: publish katyoti v1 + v2 to a file:// catalog,
// install v1, pre-place a v1 binary at the control-plane location, then `self-update`
// — the running-exe swap must leave the control-plane binary reporting v2 (proven by
// running it), with the new version also in the store and no staging leftovers.
func TestSelfUpdateSwapsControlPlaneBinary(t *testing.T) {
	pub := newInst(t)
	deploy := filepath.Join(pub.workspace, "deploy")
	d1 := buildKatyotiAt(t, filepath.Join(pub.workspace, "dist1"), "e2e-v1")
	d2 := buildKatyotiAt(t, filepath.Join(pub.workspace, "dist2"), "e2e-v2")
	channel := publishInfra(t, deploy, []string{"e2e-v1"}, []string{d1})

	// the machine installs its control plane from the channel at e2e-v1 — a
	// real binary at <root>/control-plane, so self-update SWAPS it in place.
	con := newInst(t)
	useInfra(t, con, channel)
	con.run(t, false, katyotiBin, "install-infra")
	cpBin := filepath.Join(con.root, "control-plane", "katyoti"+exeSuffix())
	if v := con.run(t, false, cpBin, "version"); !strings.Contains(v, "e2e-v1") {
		t.Fatalf("pre-swap control-plane binary should be e2e-v1:\n%s", v)
	}

	// the channel moves on: e2e-v2 is current, e2e-v1 the previous.
	publishInfra(t, deploy, []string{"e2e-v1", "e2e-v2"}, []string{d1, d2})

	// self-update → resolves latest (e2e-v2), installs into the store, swaps the
	// control-plane binary in place.
	out := con.run(t, false, katyotiBin, "self-update")
	if !strings.Contains(out, "katyoti") || !strings.Contains(out, "e2e-v2") {
		t.Fatalf("self-update should report katyoti → e2e-v2:\n%s", out)
	}

	// the control-plane binary now reports e2e-v2 — the swap landed in place.
	if v := con.run(t, false, cpBin, "version"); !strings.Contains(v, "e2e-v2") {
		t.Fatalf("post-swap control-plane binary should be e2e-v2:\n%s", v)
	}
	// the new version is in the store too.
	if _, err := os.Stat(filepath.Join(con.root, "bin", "store", "katyoti", "e2e-v2", "katyoti"+exeSuffix())); err != nil {
		t.Fatalf("e2e-v2 should be in the store: %v", err)
	}
	// a clean swap leaves no .new-/.old- staging files (nothing held the binary).
	for _, marker := range []string{".new-", ".old-"} {
		if m, _ := filepath.Glob(cpBin + marker + "*"); len(m) != 0 {
			t.Errorf("staging leftovers after swap: %v", m)
		}
	}

	// re-running self-update is a no-op now that the install record is at latest.
	if out := con.run(t, false, katyotiBin, "self-update"); !strings.Contains(out, "up to date") {
		t.Fatalf("second self-update should be a no-op:\n%s", out)
	}
}

// TestInstallInfraBootstrapsControlPlane proves the bootstrap path: from a fresh
// instance subscribed to a catalog serving katyoti, `install-infra` places katyoti
// at the control-plane location (verified byte-identical to the store binary) and
// reports ktw-hub/ktw-gui as skipped (the local catalog does not serve them).
// Idempotent on re-run. Verification is by byte-comparison, not execution, so the
// control-plane binary is never locked (no AV scan-on-exec) — the re-run's swap
// then leaves no staging leftovers.
func TestInstallInfraBootstrapsControlPlane(t *testing.T) {
	pub := newInst(t)
	d1 := buildKatyotiAt(t, filepath.Join(pub.workspace, "dist1"), "e2e-v1")
	channel := publishInfra(t, filepath.Join(pub.workspace, "deploy"), []string{"e2e-v1"}, []string{d1})

	con := newInst(t)
	useInfra(t, con, channel)

	out := con.run(t, false, katyotiBin, "install-infra")
	if !strings.Contains(out, "installed katyoti@e2e-v1") {
		t.Fatalf("install-infra should place katyoti at control-plane:\n%s", out)
	}
	if !strings.Contains(out, "ktw-hub") || !strings.Contains(out, "ktw-gui") {
		t.Fatalf("install-infra should report the unserved infra binaries as skipped:\n%s", out)
	}

	cpBin := filepath.Join(con.root, "control-plane", "katyoti"+exeSuffix())
	storeBin := filepath.Join(con.root, "bin", "store", "katyoti", "e2e-v1", "katyoti"+exeSuffix())
	if !sameBytes(t, cpBin, storeBin) {
		t.Fatalf("control-plane katyoti should be byte-identical to the store e2e-v1 binary")
	}

	// idempotent: a second run still succeeds and leaves the binary in place with
	// no staging leftovers (the binary was never executed, so nothing holds it).
	con.run(t, false, katyotiBin, "install-infra")
	if !sameBytes(t, cpBin, storeBin) {
		t.Fatalf("control-plane katyoti should still match e2e-v1 after re-run")
	}
	for _, marker := range []string{".new-", ".old-"} {
		if m, _ := filepath.Glob(cpBin + marker + "*"); len(m) != 0 {
			t.Errorf("staging leftovers after install-infra: %v", m)
		}
	}
}

// sameBytes reports whether two files have identical contents.
func sameBytes(t *testing.T, a, b string) bool {
	t.Helper()
	da, err := os.ReadFile(a)
	if err != nil {
		t.Fatalf("read %s: %v", a, err)
	}
	db, err := os.ReadFile(b)
	if err != nil {
		t.Fatalf("read %s: %v", b, err)
	}
	return bytes.Equal(da, db)
}

// --- small file helpers ------------------------------------------------------

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

func copyDir(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.Walk(from, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if fi.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o755)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

// --- multi-version publishing: accumulate, pin, refuse republish -------------

// buildSidecarAt compiles a sample SIDECAR (ktw-host — never a control-plane
// binary, which is published to the infra channel, not a catalog) stamped with
// `version` into a dist-shaped platform folder, as `make dist-all` presents it.
func buildSidecarAt(t *testing.T, root, version string) string {
	t.Helper()
	dir := filepath.Join(root, runtime.GOOS+"-"+runtime.GOARCH)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "ktw-host"+exeSuffix())
	cmd := exec.Command("go", "build",
		"-ldflags", "-X github.com/katyoti/katyoti-fwk/app/ktw-host/cmd.version="+version,
		"-o", out, "github.com/katyoti/katyoti-fwk/app/ktw-host")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build ktw-host@%s: %v", version, err)
	}
	return dir
}

// publishInfra publishes katyoti builds as an INFRA CHANNEL — where the
// control plane is installed from (never a catalog): <root>/infra/channel.yaml
// (the version pointer: the last version given, and the one before it), and per
// version <version>/<os-arch>/katyoti[.exe] + SHA256SUMS. dists are
// buildKatyotiAt folders, oldest first. It answers the channel folder.
// TestGUIMovesTheControlPlaneOnly: the GUI's Infra tab hands off to `katyoti
// gui --infra-version <v>` (update, roll back) and `katyoti gui --repair`;
// both move the control plane before the GUI starts, and nothing else. PATH is
// narrowed so no ktw-gui is found: the launch fails AFTER the move, which is
// the part under test (and no window opens on the machine running the suite).
func TestGUIMovesTheControlPlaneOnly(t *testing.T) {
	pub := newInst(t)
	deploy := filepath.Join(pub.workspace, "deploy")
	d1 := buildKatyotiAt(t, filepath.Join(pub.workspace, "dist1"), "e2e-v1")
	d2 := buildKatyotiAt(t, filepath.Join(pub.workspace, "dist2"), "e2e-v2")
	channel := publishInfra(t, deploy, []string{"e2e-v1", "e2e-v2"}, []string{d1, d2})
	con := newInst(t)
	useInfra(t, con, channel)
	con.run(t, false, katyotiBin, "install-infra")
	cpBin := filepath.Join(con.root, "control-plane", "katyoti"+exeSuffix())

	gui := func(args ...string) string {
		cmd := exec.Command(katyotiBin, append([]string{"gui"}, args...)...)
		cmd.Env = append(con.env(), "PATH="+filepath.Join(os.Getenv("SystemRoot"), "System32"))
		out, _ := cmd.CombinedOutput()
		return string(out)
	}
	out := gui("--infra-version", "e2e-v1")
	if !strings.Contains(out, "e2e-v2 → e2e-v1") {
		t.Fatalf("gui --infra-version should roll the control plane back first:\n%s", out)
	}
	if v := con.run(t, false, cpBin, "version"); !strings.Contains(v, "e2e-v1") {
		t.Fatalf("the control-plane binary should be e2e-v1 after the roll back:\n%s", v)
	}
	if out := gui("--repair"); !strings.Contains(out, "placed again") {
		t.Fatalf("gui --repair should place the control plane again:\n%s", out)
	}
	if v := con.run(t, false, cpBin, "version"); !strings.Contains(v, "e2e-v2") {
		t.Fatalf("a repair places the channel's current version (e2e-v2):\n%s", v)
	}
}

// TestInstancesSideBySide: two named instances of katyoti on one machine, each
// with its own control plane at its own version — created from the channel this
// installation uses, listed, entered by name — and nothing of either written
// into the installation they were created from.
func TestInstancesSideBySide(t *testing.T) {
	pub := newInst(t)
	deploy := filepath.Join(pub.workspace, "deploy")
	d1 := buildKatyotiAt(t, filepath.Join(pub.workspace, "dist1"), "e2e-v1")
	d2 := buildKatyotiAt(t, filepath.Join(pub.workspace, "dist2"), "e2e-v2")
	channel := publishInfra(t, deploy, []string{"e2e-v1", "e2e-v2"}, []string{d1, d2})
	host := newInst(t)
	useInfra(t, host, channel)
	folders := t.TempDir()
	registry := filepath.Join(t.TempDir(), "instances.yaml")

	run := func(wantErr bool, args ...string) string {
		cmd := exec.Command(katyotiBin, append([]string{"instance"}, args...)...)
		cmd.Env = append(host.env(), "KTW_INSTANCES="+registry)
		out, err := cmd.CombinedOutput()
		if (err != nil) != wantErr {
			t.Fatalf("instance %v: err=%v\n%s", args, err, out)
		}
		return string(out)
	}
	run(false, "create", "next", "--folder", filepath.Join(folders, "next"), "--no-shortcut")
	run(false, "create", "old", "--folder", filepath.Join(folders, "old"), "--infra", "e2e-v1", "--no-shortcut")
	run(true, "create", "next", "--folder", filepath.Join(folders, "again"), "--no-shortcut")

	list := run(false, "list")
	for _, want := range []string{"next", "e2e-v2", "old", "e2e-v1"} {
		if !strings.Contains(list, want) {
			t.Fatalf("instance list should show %q:\n%s", want, list)
		}
	}
	// Each instance runs its OWN katyoti, at its version.
	if out := run(false, "run", "old", "--", "katyoti", "version"); !strings.Contains(out, "e2e-v1") {
		t.Fatalf("the old instance's katyoti should be e2e-v1:\n%s", out)
	}
	if out := run(false, "run", "next", "--", "katyoti", "version"); !strings.Contains(out, "e2e-v2") {
		t.Fatalf("the next instance's katyoti should be e2e-v2:\n%s", out)
	}
	// Nothing written into the installation they were created from.
	for _, p := range []string{"control-plane", "sidecars.yaml", "marketplaces.yaml"} {
		if _, err := os.Stat(filepath.Join(host.root, p)); err == nil {
			t.Errorf("an instance wrote %s into the installation it was created from", p)
		}
	}
	for _, name := range []string{"next", "old"} {
		if _, err := os.Stat(filepath.Join(folders, name, "root", "control-plane", "katyoti"+exeSuffix())); err != nil {
			t.Errorf("instance %s should hold its own control plane: %v", name, err)
		}
	}
	if out := run(false, "remove", "old"); !strings.Contains(out, "kept") {
		t.Fatalf("remove should say the folder is kept:\n%s", out)
	}
	if out := run(false, "list"); strings.Contains(out, "old") {
		t.Fatalf("a removed instance is still listed:\n%s", out)
	}
}

// TestTeamMarketplaceEndToEnd: a team's own marketplace, authored with the
// compose verbs — init (its key under its own prefix), the infra channel built
// in Go — and installed on a fresh machine by `setup`: the channel pinned to
// the key the pointer names, the catalog subscribed under the marketplace's own
// name, never « katyoti ».
func TestTeamMarketplaceEndToEnd(t *testing.T) {
	pub := newInst(t)
	market := filepath.Join(pub.workspace, "acme-market")
	keys := filepath.Join(pub.workspace, "keys")
	pub.run(t, false, composeBin, "init-marketplace", market, "--name", "acme", "--key-folder", keys, "--key-prefix", "acme")
	dist := filepath.Dir(buildKatyotiAt(t, filepath.Join(pub.workspace, "dist"), "e2e-team-v1"))
	out := pub.run(t, false, composeBin, "build-infra-channel", dist, "--output", market, "--version", "e2e-team-v1",
		"--key-folder", keys, "--key-prefix", "acme")
	if !strings.Contains(out, "e2e-team-v1") {
		t.Fatalf("build-infra-channel should report the version:\n%s", out)
	}
	for _, f := range []string{"infra/channel.yaml.minisig", "infra/install.ps1", "acme.key.pub"} {
		if _, err := os.Stat(filepath.Join(market, f)); err != nil {
			t.Fatalf("the marketplace should hold %s: %v", f, err)
		}
	}

	con := newInst(t)
	cmd := exec.Command(katyotiBin, "setup", "--no-gui")
	cmd.Env = append(con.env(), "KTW_INFRA_CHANNEL="+filepath.Join(market, "infra"))
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("setup: %v\n%s", err, b)
	}
	got := string(b)
	if !strings.Contains(got, "pinned") || !strings.Contains(got, `marketplace "acme" subscribed`) {
		t.Fatalf("setup should pin the team's key and subscribe the catalog as acme:\n%s", got)
	}
	if list := con.run(t, false, katyotiBin, "marketplace", "list"); !strings.Contains(list, "acme") || !strings.Contains(list, "pinned") {
		t.Fatalf("the subscription should be acme, pinned:\n%s", list)
	}
}

// TestSetupInstallsTheChannelsSolution: a marketplace whose infra channel
// names a solution (`build-infra-channel --demo`, written `solution:`);
// `katyoti setup` on a fresh machine installs it under its own name — the
// universe « demo-rel », which records it, its project in the person's
// Documents, the default universe — and again it repairs, keeping the
// project; `--no-solution` (and its former name `--no-demo`) skips it. No
// program is installed by setup.
func TestSetupInstallsTheChannelsSolution(t *testing.T) {
	pub := newInst(t)
	market := filepath.Join(pub.workspace, "acme-market")
	catalog := filepath.Join(market, "katyoti-marketplace.yaml")
	keys := filepath.Join(pub.workspace, "keys")
	key := []string{"--key-folder", keys, "--key-prefix", "acme"}
	pub.run(t, false, composeBin, "init-marketplace", market, "--name", "acme", "--key-folder", keys, "--key-prefix", "acme")
	content := filepath.Join(pub.workspace, "content")
	pub.run(t, false, composeBin, "create-content", "flowpack", "--output", content)
	pub.run(t, false, composeBin, append([]string{"build-content", filepath.Join(content, "flowpack"), "--version", "1.0.0", "--output", market}, key...)...)

	src := newInst(t)
	writeFile(t, filepath.Join(src.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\ndisplay_name: Demo\n")
	src.run(t, false, katyotiBin, "marketplace", "add", "acme", catalog)
	src.run(t, false, katyotiBin, "install", "flowpack")
	src.run(t, false, katyotiBin, "universe", "wire", "flowpack")
	project := filepath.Join(t.TempDir(), "demo-src-project")
	writeFile(t, filepath.Join(project, "README.md"), "# The demo\n")
	writeFile(t, filepath.Join(project, "modules", "demo", "bienvenue.md"), "# Bienvenue\n")
	src.run(t, false, composeBin, append([]string{"snapshot-universe", src.workspace, "--version", "v1", "--marketplace", market,
		"--name", "demo-rel", "--project", project}, key...)...)

	dist := filepath.Dir(buildKatyotiAt(t, filepath.Join(pub.workspace, "dist"), "e2e-demo-v1"))
	pub.run(t, false, composeBin, append([]string{"build-infra-channel", dist, "--output", market, "--version", "e2e-demo-v1", "--demo", "demo-rel@v1"}, key...)...)

	con := newInst(t)
	userHome := t.TempDir()
	setup := func(args ...string) string {
		cmd := exec.Command(katyotiBin, append([]string{"setup", "--no-gui"}, args...)...)
		cmd.Env = append(con.env(), "KTW_INFRA_CHANNEL="+filepath.Join(market, "infra"), "USERPROFILE="+userHome, "HOME="+userHome)
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, b)
		}
		return string(b)
	}
	out := setup()
	solProject := filepath.Join(userHome, "Documents", "Katyoti", "demo-rel")
	if !strings.Contains(out, "solution demo-rel@v1 installed: universe demo-rel,") {
		t.Fatalf("setup should install the channel's solution under its own name:\n%s", out)
	}
	if strings.Contains(out, "prereqs install") || strings.Contains(out, "installing ") {
		t.Fatalf("setup should install no program:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(solProject, "modules", "demo", "bienvenue.md")); err != nil {
		t.Fatalf("the solution's project should be in the person's Documents: %v\n%s", err, out)
	}
	if list := con.run(t, false, katyotiBin, "universe", "list"); !strings.Contains(list, "demo-rel") {
		t.Fatalf("the universe demo-rel should be registered:\n%s", list)
	}
	if cfg := con.run(t, false, katyotiBin, "config", "get"); !strings.Contains(cfg, "demo-rel") {
		t.Fatalf("the solution's main universe should be the default:\n%s", cfg)
	}
	// The universe records the solution that installed it.
	if yml, err := os.ReadFile(filepath.Join(con.root, "universes", "demo-rel", ".katyoti", "config", "Univers.yaml")); err != nil || !strings.Contains(string(yml), "solution: demo-rel@v1") {
		t.Fatalf("the universe should record its solution: %v\n%s", err, yml)
	}
	// Again: it repairs and keeps the person's project.
	writeFile(t, filepath.Join(solProject, "mine.md"), "my notes\n")
	if out := setup(); !strings.Contains(out, "installed: universe demo-rel,") {
		t.Fatalf("setup again should repair the solution:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(solProject, "mine.md")); err != nil {
		t.Fatal("setup again lost the person's file in the solution's project")
	}
	for _, flag := range []string{"--no-solution", "--no-demo"} {
		if out := setup(flag); strings.Contains(out, "solution demo-rel") {
			t.Fatalf("%s should skip the solution:\n%s", flag, out)
		}
	}
}

// TestSolutionWithVaultsAndDocs: a solution pins two config-sets and a vault
// and delivers documents in two languages and « Quoi de neuf »; the
// marketplace keeps each kind in its folder (config-sets/, vaults/,
// solutions/<name>/<v>/). Installing it without the vault's password installs
// nothing; with it, the vault's keys land in their scopes, both config-sets
// apply, and the documents are in the project. A vault imports alone too.
func TestSolutionWithVaultsAndDocs(t *testing.T) {
	pub := newInst(t)
	market := filepath.Join(pub.workspace, "market")
	catalog := filepath.Join(market, "katyoti-marketplace.yaml")
	t.Setenv("KTW_E2E_VAULT_PASSWORD", "the team's vault password")

	// Two config-sets: one key in the universe, one global.
	for _, cs := range []struct{ name, key, scope string }{{"cs-a", "A_TOKEN", "universe"}, {"cs-b", "B_URL", "global"}} {
		dir := filepath.Join(pub.workspace, cs.name)
		writeFile(t, filepath.Join(dir, "keys.yaml"), "apiVersion: 1\nkeys:\n  - name: "+cs.key+"\n    scope: "+cs.scope+"\n")
		writeFile(t, filepath.Join(dir, "config", strings.ToUpper(cs.name[3:])+".yaml"), "v: ${{env:"+cs.key+"}}\n")
		pub.run(t, false, composeBin, "build-config-set", dir, "--name", cs.name, "--version", "1.0.0", "--output", market)
	}
	// A vault: the team's keys, exported from where they are set.
	writeFile(t, filepath.Join(pub.workspace, ".katyoti", ".env"), "A_TOKEN=a-secret\n")
	writeFile(t, filepath.Join(pub.root, ".env"), "B_URL=https://b.example\n")
	manifest := filepath.Join(pub.workspace, "team-keys.yaml")
	writeFile(t, manifest, "keys:\n  - name: A_TOKEN\n  - name: B_URL\n    scope: global\n")
	sealed := filepath.Join(pub.workspace, "team.keys")
	pub.run(t, false, katyotiBin, "keys", "export", sealed, "--universe", pub.workspace, "--manifest", manifest, "--yes", "--password-env", "KTW_E2E_VAULT_PASSWORD")
	pub.run(t, false, composeBin, "build-vault", sealed, "--name", "team-keys", "--version", "v1", "--output", market)
	// A member, and the universe the solution is made from.
	content := filepath.Join(pub.workspace, "content")
	pub.run(t, false, composeBin, "create-content", "flowpack", "--output", content)
	pub.run(t, false, composeBin, "build-content", filepath.Join(content, "flowpack"), "--version", "1.0.0", "--output", market)
	src := newInst(t)
	writeFile(t, filepath.Join(src.workspace, ".katyoti", "config", "Univers.yaml"), "name: sol\ndisplay_name: Sol\n")
	src.run(t, false, katyotiBin, "marketplace", "add", "m", catalog)
	src.run(t, false, katyotiBin, "install", "flowpack")
	src.run(t, false, katyotiBin, "universe", "wire", "flowpack")
	project := filepath.Join(t.TempDir(), "sol-project")
	writeFile(t, filepath.Join(project, "modules", "demo", "objects", "demo.cwm.yaml"), "id: demo\ntype: module\nname: Demo\n")
	docs := filepath.Join(t.TempDir(), "docs")
	writeFile(t, filepath.Join(docs, "fr", "lisez-moi.md"), "# Lisez-moi\n\nBonjour.\n")
	writeFile(t, filepath.Join(docs, "en", "readme.md"), "# Read me\n\nHello.\n")
	changelog := filepath.Join(t.TempDir(), "9.001.md")
	writeFile(t, changelog, "# J\n\n## En un coup d'œil\n\n- Tout.\n\n## Interface\n\n### 1. Un écran\n\n**Ce que ça vous apporte :** du neuf.\n")
	out := src.run(t, false, composeBin, "snapshot-universe", src.workspace, "--version", "v1", "--marketplace", market,
		"--name", "sol", "--config-set", "cs-a@1.0.0,cs-b@1.0.0", "--vault", "team-keys@v1",
		"--project", project, "--docs", docs, "--whats-new", changelog)
	if !strings.Contains(out, `"documents": 3`) {
		t.Fatalf("the solution should deliver 3 documents:\n%s", out)
	}
	for _, f := range []string{"config-sets/1.0.0/cs-a.tar.gz", "vaults/v1/team-keys.vault", "solutions/sol/v1/solution.yaml",
		"solutions/sol/v1/docs/fr/lisez-moi.md", "solutions/sol/v1/docs/en/readme.md", "solutions/sol/v1/docs/fr/quoi-de-neuf.md"} {
		if _, err := os.Stat(filepath.Join(market, filepath.FromSlash(f))); err != nil {
			t.Errorf("the marketplace should hold %s: %v", f, err)
		}
	}

	con := newInst(t)
	con.run(t, false, katyotiBin, "marketplace", "add", "m", catalog)
	show := con.run(t, false, katyotiBin, "solution", "show", "sol")
	for _, want := range []string{"config-set  cs-a@1.0.0", "config-set  cs-b@1.0.0", "vault       team-keys@v1", "document    fr/quoi-de-neuf.md"} {
		if !strings.Contains(show, want) {
			t.Fatalf("solution show should list %q:\n%s", want, show)
		}
	}
	home := filepath.Join(t.TempDir(), "sol-uni")
	t.Setenv("KTW_E2E_WRONG_PASSWORD", "not-the-password")
	if out := con.run(t, true, katyotiBin, "solution", "install", "sol", "--universe", home, "--non-interactive", "--vault-password-env", "KTW_E2E_WRONG_PASSWORD"); !strings.Contains(out, "team-keys not opened") {
		t.Fatalf("a vault's wrong password should be refused:\n%s", out)
	}
	// No password: the vault stays closed (imported later), and a key the
	// universe cannot do without is refused by the gate.
	if out := con.run(t, true, katyotiBin, "solution", "install", "sol", "--universe", home, "--non-interactive"); !strings.Contains(out, "A_TOKEN") {
		t.Fatalf("without the vault, a key the universe needs should be refused:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(con.root, "bin", "store-content", "flowpack")); err == nil {
		t.Fatal("a refused install installed a member")
	}
	person := t.TempDir()
	out = con.withHome(person).run(t, false, katyotiBin, "solution", "install", "sol", "--universe", home, "--non-interactive", "--vault-password-env", "KTW_E2E_VAULT_PASSWORD")
	if !strings.Contains(out, "vault team-keys@v1: 2 key(s)") || strings.Count(out, "all keys set") != 2 {
		t.Fatalf("the install should import the vault and apply both config-sets:\n%s", out)
	}
	if env := readFileString(t, filepath.Join(home, ".katyoti", ".env")); !strings.Contains(env, "A_TOKEN=a-secret") {
		t.Fatalf("the vault's universe key should be in the universe .env:\n%s", env)
	}
	if env := readFileString(t, filepath.Join(con.root, ".env")); !strings.Contains(env, "B_URL=https://b.example") {
		t.Fatalf("the vault's global key should be in the global .env:\n%s", env)
	}
	// The seed's project, in the projects folder under the universe's name.
	proj := filepath.Join(person, "Documents", "Katyoti", "sol-uni")
	for _, f := range []string{"lisez-moi-fr.cwm.md", "readme-en.cwm.md", "quoi-de-neuf-fr.cwm.md"} {
		if _, err := os.Stat(filepath.Join(proj, "modules", "demo", "objects", f)); err != nil {
			t.Errorf("the document %s should be in the project: %v\n%s", f, err, out)
		}
	}
	// A vault alone.
	other := newInst(t)
	other.run(t, false, katyotiBin, "marketplace", "add", "m", catalog)
	if out := other.run(t, false, katyotiBin, "vault", "import", "team-keys", "--universe", other.workspace, "--password-env", "KTW_E2E_VAULT_PASSWORD"); !strings.Contains(out, "A_TOKEN") {
		t.Fatalf("vault import should write its keys:\n%s", out)
	}
}

func publishInfra(t *testing.T, root string, versions []string, dists []string) string {
	t.Helper()
	ch := filepath.Join(root, "infra")
	plat := runtime.GOOS + "-" + runtime.GOARCH
	for i, v := range versions {
		dir := filepath.Join(ch, v, plat)
		bin := "katyoti" + exeSuffix()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		copyFile(t, filepath.Join(dists[i], bin), filepath.Join(dir, bin))
		data, err := os.ReadFile(filepath.Join(dir, bin))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		writeFile(t, filepath.Join(dir, "SHA256SUMS"), hex.EncodeToString(sum[:])+"  "+bin+"\n")
	}
	pointer := "version: " + versions[len(versions)-1] + "\n"
	if len(versions) > 1 {
		pointer += "previous: " + versions[len(versions)-2] + "\n"
	}
	writeFile(t, filepath.Join(ch, "channel.yaml"), pointer)
	return ch
}

// useInfra records the channel a machine installs its control plane from (its
// <root>/infra-channel.yaml, as setup writes it).
func useInfra(t *testing.T, in inst, channel string) {
	t.Helper()
	writeFile(t, filepath.Join(in.root, "infra-channel.yaml"), "root: "+filepath.ToSlash(channel)+"\n")
}

// buildKatyotiAt compiles the katyoti binary stamped with `version` into a
// dist-shaped platform folder, so `release-marketplace` sees it the way a real
// `make dist-all` tree presents it.
func buildKatyotiAt(t *testing.T, root, version string) string {
	t.Helper()
	dir := filepath.Join(root, runtime.GOOS+"-"+runtime.GOARCH)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "katyoti"+exeSuffix())
	cmd := exec.Command("go", "build",
		"-ldflags", "-X github.com/katyoti/katyoti-fwk/app/katyoti/cmd.version="+version,
		"-o", out, "github.com/katyoti/katyoti-fwk/app/katyoti")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build katyoti@%s: %v", version, err)
	}
	return dir
}

// TestMultiVersionPublish proves publishing a second version ADDS to the
// marketplace instead of replacing it: both versions stay installable from one
// URL, an explicit @version pins the old one, a bare install takes the newest,
// and republishing an existing version is refused.
//
// This is the regression guard for the whole accumulate model — before it,
// publishing rebuilt the catalog from scratch and the previous version
// simply vanished.
func TestMultiVersionPublish(t *testing.T) {
	pub := newInst(t)
	market := filepath.Join(pub.workspace, "market")

	// --- release 1 ---
	d1 := buildSidecarAt(t, filepath.Join(pub.workspace, "dist1"), "e2e-v1")
	pub.run(t, false, composeBin, "release-marketplace", d1,
		"--output", market, "--name", "demo", "--keep", "3")

	// --- release 2 ---
	d2 := buildSidecarAt(t, filepath.Join(pub.workspace, "dist2"), "e2e-v2")
	pub.run(t, false, composeBin, "release-marketplace", d2,
		"--output", market, "--name", "demo", "--keep", "3")

	catalog := filepath.Join(market, "katyoti-marketplace.yaml")
	cat, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"e2e-v1", "e2e-v2"} {
		if !strings.Contains(string(cat), want) {
			t.Fatalf("catalog lost %s — publishing must accumulate, not replace:\n%s", want, cat)
		}
	}
	// Assets must be at VERSION-FIRST paths (`sidecars/<version>/<arch>/<file>`), or v1's
	// recorded sha256 would point at bytes v2 overwrote.
	if !strings.Contains(string(cat), "url: sidecars/e2e-v1/") || !strings.Contains(string(cat), "url: sidecars/e2e-v2/") {
		t.Fatalf("asset URLs must lead with the version:\n%s", cat)
	}
	// ...and must carry NO per-item folder prefix: the filename already identifies
	// the item, and version-first is what makes retiring a whole release a single
	// `rm -rf <version>/`.
	if strings.Contains(string(cat), "url: sidecars/ktw-host/") {
		t.Fatalf("asset URLs must not be prefixed by the item folder:\n%s", cat)
	}

	// --- republishing an existing version is refused ---
	out := pub.run(t, true, composeBin, "release-marketplace", d2,
		"--output", market, "--name", "demo", "--keep", "3")
	if !strings.Contains(out, "immutable") && !strings.Contains(out, "already published") {
		t.Fatalf("republishing an existing version must be refused:\n%s", out)
	}

	// --- consumer: both versions installable from ONE url ---
	con := newInst(t)
	con.run(t, false, katyotiBin, "marketplace", "add", "demo", fileURL(catalog))

	con.run(t, false, katyotiBin, "install", "ktw-host@e2e-v1")
	con.run(t, false, katyotiBin, "install", "ktw-host@e2e-v2")
	installed := con.run(t, false, katyotiBin, "installed")
	if !strings.Contains(installed, "ktw-host") {
		t.Fatalf("installs should be recorded:\n%s", installed)
	}

	// The store is version-addressed, so both sets of bytes coexist.
	for _, v := range []string{"e2e-v1", "e2e-v2"} {
		p := filepath.Join(con.root, "bin", "store", "ktw-host", v, "ktw-host"+exeSuffix())
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected %s in the versioned store: %v", p, err)
		}
	}
}

// TestRemoveVersionAndPerVersionUninstall covers the delete-version verbs end to
// end: publisher withdraws ONE published version (catalog + assets + re-sign, with
// a pinned consumer still verifying afterwards), and the consumer retires ONE store
// version while keeping the rest.
func TestRemoveVersionAndPerVersionUninstall(t *testing.T) {
	pub := newInst(t)
	market := filepath.Join(pub.workspace, "market")
	pub.run(t, false, composeBin, "keygen", "--folder", "keys", "--prefix", "team")

	d1 := buildSidecarAt(t, filepath.Join(pub.workspace, "dist1"), "e2e-v1")
	pub.run(t, false, composeBin, "release-marketplace", d1, "--output", market, "--name", "demo", "--keep", "5")
	d2 := buildSidecarAt(t, filepath.Join(pub.workspace, "dist2"), "e2e-v2")
	pub.run(t, false, composeBin, "release-marketplace", d2, "--output", market, "--name", "demo", "--keep", "5")
	catalog := filepath.Join(market, "katyoti-marketplace.yaml")
	pub.run(t, false, composeBin, "sign-marketplace", catalog, "--key-folder", "keys", "--key-prefix", "team")

	// Both versions live at VERSION-FIRST paths, sharing the platform folder.
	v1Asset := filepath.Join(market, "sidecars", "e2e-v1", runtime.GOOS+"-"+runtime.GOARCH, "ktw-host"+exeSuffix())
	v2Asset := filepath.Join(market, "sidecars", "e2e-v2", runtime.GOOS+"-"+runtime.GOARCH, "ktw-host"+exeSuffix())
	for _, p := range []string{v1Asset, v2Asset} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected asset at the version-first path %s: %v", p, err)
		}
	}

	// --- consumer subscribes with PINNED trust and installs BOTH versions ---
	pubkey := filepath.Join(pub.workspace, "keys", "team.key.pub")
	con := newInst(t)
	con.run(t, false, katyotiBin, "marketplace", "add", "demo", fileURL(catalog), "--pubkey", pubkey, "--trust", "pinned")
	con.run(t, false, katyotiBin, "install", "ktw-host@e2e-v1")
	con.run(t, false, katyotiBin, "install", "ktw-host@e2e-v2") // current becomes e2e-v2
	for _, v := range []string{"e2e-v1", "e2e-v2"} {
		if _, err := os.Stat(filepath.Join(con.root, "bin", "store", "ktw-host", v, "ktw-host"+exeSuffix())); err != nil {
			t.Fatalf("%s should be in the store: %v", v, err)
		}
	}

	// --- publisher withdraws e2e-v1 ---
	out := pub.run(t, false, composeBin, "remove-version", "ktw-host@e2e-v1",
		"--output", market, "--key-folder", "keys", "--key-prefix", "team")
	if !strings.Contains(out, "e2e-v1") {
		t.Fatalf("remove-version should report what it withdrew: %s", out)
	}
	// v1 bytes gone, v1 folder GC'd, v2 untouched (the shared platform folder
	// must not take the surviving version with it).
	if _, err := os.Stat(v1Asset); !os.IsNotExist(err) {
		t.Errorf("withdrawn asset should be deleted, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(market, "e2e-v1")); !os.IsNotExist(err) {
		t.Errorf("emptied version folder should be GC'd, got err=%v", err)
	}
	if _, err := os.Stat(v2Asset); err != nil {
		t.Fatalf("the retained version was destroyed: %v", err)
	}
	cat, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cat), "e2e-v1") {
		t.Errorf("catalog still advertises the withdrawn version: %s", cat)
	}

	// --- the pinned consumer still verifies: remove-version RE-SIGNED ---
	con.run(t, false, katyotiBin, "marketplace", "refresh", "demo")

	// --- consumer retires ONE store version ---
	out = con.run(t, false, katyotiBin, "uninstall", "ktw-host@e2e-v1")
	if !strings.Contains(out, "other versions kept") {
		t.Fatalf("per-version uninstall should say the rest are kept: %s", out)
	}
	if _, err := os.Stat(filepath.Join(con.root, "bin", "store", "ktw-host", "e2e-v1")); !os.IsNotExist(err) {
		t.Errorf("e2e-v1 should be gone from the store, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(con.root, "bin", "store", "ktw-host", "e2e-v2", "ktw-host"+exeSuffix())); err != nil {
		t.Fatalf("the current version must survive a per-version uninstall: %v", err)
	}

	// --- the current version is protected from a per-version uninstall ---
	out = con.run(t, true, katyotiBin, "uninstall", "ktw-host@e2e-v2")
	if !strings.Contains(out, "current version") {
		t.Fatalf("uninstalling the current version must be refused with a clear reason: %s", out)
	}
}

// TestConfigSetPublishInstallShow covers the config-set packaging half end to end:
// a publisher packages a bundle (refusing one with an undeclared ${VAR}), a consumer
// installs it into the config store, and `config-set list/show` report its versions
// and declared keys. A tampered bundle is refused by the sha256 pin like any item.
func TestConfigSetPublishInstallShow(t *testing.T) {
	pub := newInst(t)
	market := filepath.Join(pub.workspace, "market")

	// --- a VALID bundle: every ${VAR} in a template is declared ---
	good := filepath.Join(pub.workspace, "northwind-base")
	writeFile(t, filepath.Join(good, "keys.yaml"), `apiVersion: 1
keys:
  - name: LEANIX_API_TOKEN
    description: LeanIX API token
    secret: true
  - name: CONFLUENCE_BASE_URL
    description: Confluence base URL
    scope: global
`)
	writeFile(t, filepath.Join(good, "config", "LeanIX.yaml"), "token: ${{env:LEANIX_API_TOKEN}}\n")
	writeFile(t, filepath.Join(good, "config", "Confluence.yaml"), "base: ${{env:CONFLUENCE_BASE_URL}}\n")

	out := pub.run(t, false, composeBin, "build-config-set", good,
		"--name", "northwind-base", "--version", "1.0.0", "--output", market,
		"--description", "NORTHWIND base configuration")
	if !strings.Contains(out, "northwind-base") {
		t.Fatalf("build-config-set should report the item: %s", out)
	}
	// The bundle lands in its own configs/ folder, version first like every asset.
	asset := filepath.Join(market, "config-sets", "1.0.0", "northwind-base.tar.gz")
	if _, err := os.Stat(asset); err != nil {
		t.Fatalf("bundle should be at configs/<version>/<name>.tar.gz: %v", err)
	}

	// --- an INVALID bundle is refused at PUBLISH time ---
	bad := filepath.Join(pub.workspace, "bad-set")
	writeFile(t, filepath.Join(bad, "keys.yaml"), "apiVersion: 1\nkeys:\n  - name: DECLARED\n")
	writeFile(t, filepath.Join(bad, "config", "Thing.yaml"), "a: ${{env:DECLARED}}\nb: ${{env:NEVER_DECLARED}}\n")
	out = pub.run(t, true, composeBin, "build-config-set", bad,
		"--name", "bad-set", "--version", "1.0.0", "--output", market)
	if !strings.Contains(out, "NEVER_DECLARED") {
		t.Fatalf("an undeclared ${VAR} must be refused, naming the key: %s", out)
	}

	// --- the manifest's former name is refused at publish, saying to rename it ---
	legacy := filepath.Join(pub.workspace, "legacy-set")
	writeFile(t, filepath.Join(legacy, "required-keys.yaml"), "apiVersion: 1\nkeys:\n  - name: DECLARED\n")
	writeFile(t, filepath.Join(legacy, "config", "Thing.yaml"), "a: ${{env:DECLARED}}\n")
	out = pub.run(t, true, composeBin, "build-config-set", legacy,
		"--name", "legacy-set", "--version", "1.0.0", "--output", market)
	if !strings.Contains(out, "keys.yaml") {
		t.Fatalf("a bundle with only required-keys.yaml must be refused, naming keys.yaml: %s", out)
	}

	// --- consumer: subscribe, list, install, show ---
	catalog := filepath.Join(market, "katyoti-marketplace.yaml")
	con := newInst(t)
	con.run(t, false, katyotiBin, "marketplace", "add", "demo", fileURL(catalog))

	out = con.run(t, false, katyotiBin, "config-set", "list")
	if !strings.Contains(out, "northwind-base") {
		t.Fatalf("config-set list should show the published set: %s", out)
	}

	// Before install, keys are unreadable (they ship inside the bundle) — the
	// command must SAY so rather than print an empty key list.
	out = con.run(t, false, katyotiBin, "config-set", "show", "northwind-base")
	if !strings.Contains(out, "not installed") {
		t.Fatalf("show on an uninstalled set must say the keys are not readable yet: %s", out)
	}

	con.run(t, false, katyotiBin, "install", "northwind-base")
	unpacked := filepath.Join(con.root, "bin", "store-config", "northwind-base", "1.0.0", "keys.yaml")
	if _, err := os.Stat(unpacked); err != nil {
		t.Fatalf("install should unpack the bundle into store-config: %v", err)
	}

	out = con.run(t, false, katyotiBin, "config-set", "show", "northwind-base")
	for _, want := range []string{"LEANIX_API_TOKEN", "CONFLUENCE_BASE_URL", "global"} {
		if !strings.Contains(out, want) {
			t.Fatalf("show should list the declared keys + scopes (missing %q): %s", want, out)
		}
	}

	// --- a tampered bundle is refused by the sha256 pin ---
	if err := os.WriteFile(asset, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	fresh := newInst(t)
	fresh.run(t, false, katyotiBin, "marketplace", "add", "demo", fileURL(catalog))
	if out := fresh.run(t, true, katyotiBin, "install", "northwind-base"); !strings.Contains(out, "sha256 mismatch") {
		t.Fatalf("a tampered config-set bundle must be refused: %s", out)
	}
}

// TestConfigSetApply covers the core UX win: applying a config-set drops its
// templates into a universe and records EVERY key it needs in one pass, into the
// correctly scoped .env. Then proves re-apply is idempotent — the property that
// makes this safe to re-run rather than a one-shot.
func TestConfigSetApply(t *testing.T) {
	pub := newInst(t)
	market := filepath.Join(pub.workspace, "market")

	src := filepath.Join(pub.workspace, "northwind-base")
	writeFile(t, filepath.Join(src, "keys.yaml"), `apiVersion: 1
keys:
  - name: LEANIX_API_TOKEN
    description: LeanIX API token
    secret: true
    scope: universe
  - name: CONFLUENCE_BASE_URL
    description: Confluence base URL
    scope: global
  - name: LEANIX_PROXY
    description: Proxy, when the network needs one
    optional: true
`)
	writeFile(t, filepath.Join(src, "config", "LeanIX.yaml"), "token: ${{env:LEANIX_API_TOKEN}}\n")
	writeFile(t, filepath.Join(src, "config", "Confluence.yaml"), "base: ${{env:CONFLUENCE_BASE_URL}}\n")
	pub.run(t, false, composeBin, "build-config-set", src,
		"--name", "northwind-base", "--version", "1.0.0", "--output", market)

	catalog := filepath.Join(market, "katyoti-marketplace.yaml")
	con := newInst(t)
	con.run(t, false, katyotiBin, "marketplace", "add", "demo", fileURL(catalog))

	// --- a FIRST apply with only ONE of the two required keys is REFUSED,
	// naming the other, and writes NOTHING: no template, no key.
	out := con.run(t, true, katyotiBin, "config-set", "apply", "northwind-base",
		"--universe", con.workspace, "--non-interactive",
		"--set", "LEANIX_API_TOKEN=tok-123")
	if !strings.Contains(out, "CONFLUENCE_BASE_URL") || !strings.Contains(out, "nothing was written") {
		t.Fatalf("a partially-keyed apply must be refused, naming what is missing: %s", out)
	}
	if strings.Contains(out, "LEANIX_PROXY") {
		t.Fatalf("an optional key must not block an apply: %s", out)
	}
	cfgDir := filepath.Join(con.workspace, ".katyoti", "config")
	if _, err := os.Stat(filepath.Join(cfgDir, "LeanIX.yaml")); err == nil {
		t.Fatal("a refused apply copied a template")
	}
	if b, _ := os.ReadFile(filepath.Join(con.workspace, ".katyoti", ".env")); strings.Contains(string(b), "tok-123") {
		t.Fatal("a refused apply wrote a key")
	}

	// --- supply the rest: now it must succeed, the optional key left unset ---
	out = con.run(t, false, katyotiBin, "config-set", "apply", "northwind-base",
		"--universe", con.workspace, "--non-interactive",
		"--set", "LEANIX_API_TOKEN=tok-123",
		"--set", "CONFLUENCE_BASE_URL=https://confluence.example")
	if !strings.Contains(out, "all keys set") {
		t.Fatalf("a fully-keyed apply should report success: %s", out)
	}

	// Templates land in the universe's config dir (.katyoti/config), not beside it.
	for _, f := range []string{"LeanIX.yaml", "Confluence.yaml"} {
		if _, err := os.Stat(filepath.Join(cfgDir, f)); err != nil {
			t.Fatalf("template %s should be copied into the universe config dir: %v", f, err)
		}
	}

	// Each key must land in the .env its SCOPE names.
	uniEnv := readFileString(t, filepath.Join(con.workspace, ".katyoti", ".env"))
	if !strings.Contains(uniEnv, "LEANIX_API_TOKEN=tok-123") {
		t.Errorf("universe-scoped key should be in the universe .env:\n%s", uniEnv)
	}
	if strings.Contains(uniEnv, "CONFLUENCE_BASE_URL") {
		t.Errorf("a global-scoped key must NOT land in the universe .env:\n%s", uniEnv)
	}
	globalEnv := readFileString(t, filepath.Join(con.root, ".env"))
	if !strings.Contains(globalEnv, "CONFLUENCE_BASE_URL=https://confluence.example") {
		t.Errorf("global-scoped key should be in the global .env:\n%s", globalEnv)
	}

	// --- idempotence: re-apply changes nothing and skips existing templates ---
	out = con.run(t, false, katyotiBin, "config-set", "apply", "northwind-base",
		"--universe", con.workspace, "--non-interactive",
		"--set", "LEANIX_API_TOKEN=tok-123",
		"--set", "CONFLUENCE_BASE_URL=https://confluence.example")
	if !strings.Contains(out, "already present") {
		t.Errorf("re-apply must report the existing templates as skipped: %s", out)
	}
	if strings.Contains(out, "keys written") {
		t.Errorf("re-apply with identical values must write nothing: %s", out)
	}
	if again := readFileString(t, filepath.Join(con.workspace, ".katyoti", ".env")); again != uniEnv {
		t.Errorf("re-apply rewrote the .env:\nbefore:\n%s\nafter:\n%s", uniEnv, again)
	}

	// --- an unrelated key already in the .env must survive an apply ---
	envPath := filepath.Join(con.workspace, ".katyoti", ".env")
	appendFile(t, envPath, "UNRELATED=keepme\n")
	con.run(t, false, katyotiBin, "config-set", "apply", "northwind-base",
		"--universe", con.workspace, "--non-interactive",
		"--set", "LEANIX_API_TOKEN=rotated",
		"--set", "CONFLUENCE_BASE_URL=https://confluence.example")
	final := readFileString(t, envPath)
	if !strings.Contains(final, "UNRELATED=keepme") {
		t.Errorf("apply destroyed an unrelated key:\n%s", final)
	}
	if !strings.Contains(final, "LEANIX_API_TOKEN=rotated") {
		t.Errorf("apply should have rotated the value:\n%s", final)
	}
}

func readFileString(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// TestBootstrapWithConfigSet closes the config-set loop: a release can SHIP a
// config-set, and `bootstrap` then yields a universe that is not merely wired but
// KEYED — one command. Also guards the compatibility promise: a release WITHOUT a
// config-set must behave exactly as before.
func TestBootstrapWithConfigSet(t *testing.T) {
	pub := newInst(t)
	market := filepath.Join(pub.workspace, "market")

	// A content member, so the release has something real to install.
	archive, sha := contentPack(t, pub.workspace, "flowpack")
	contentCat := filepath.Join(pub.workspace, "content.yaml")
	writeFile(t, contentCat, fmt.Sprintf(`apiVersion: 1
marketplace: {name: content}
items:
  - name: flowpack
    kind: content
    versions: [{version: "1.0.0", assets: [{url: "%s", sha256: "%s"}]}]
`, fileURL(archive), sha))

	// The config-set the release will ship.
	csrc := filepath.Join(pub.workspace, "northwind-base")
	writeFile(t, filepath.Join(csrc, "keys.yaml"), `apiVersion: 1
keys:
  - name: LEANIX_API_TOKEN
    description: LeanIX API token
    secret: true
`)
	writeFile(t, filepath.Join(csrc, "config", "LeanIX.yaml"), "token: ${{env:LEANIX_API_TOKEN}}\n")
	pub.run(t, false, composeBin, "build-config-set", csrc,
		"--name", "northwind-base", "--version", "1.0.0", "--output", market)

	// A source universe with the member wired, snapshotted WITH the config-set.
	src := newInst(t)
	writeFile(t, filepath.Join(src.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\n")
	src.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	src.run(t, false, katyotiBin, "install", "flowpack")
	src.run(t, false, katyotiBin, "universe", "wire", "flowpack")
	src.run(t, false, composeBin, "snapshot-universe", src.workspace,
		"--version", "v1", "--marketplace", filepath.Join(market, "katyoti-marketplace.yaml"), "--name", "demo-rel",
		"--config-set", "northwind-base@1.0.0")

	catalog := filepath.Join(market, "katyoti-marketplace.yaml")
	if cat := readFileString(t, catalog); !strings.Contains(cat, "configSets") {
		t.Fatalf("the solution version should record its configSets:\n%s", cat)
	}

	// --- bootstrap into a FRESH instance, supplying the key non-interactively ---
	tgt := newInst(t)
	writeFile(t, filepath.Join(tgt.workspace, ".katyoti", "config", "Univers.yaml"), "name: reproduced\n")
	tgt.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	tgt.run(t, false, katyotiBin, "marketplace", "add", "releases", catalog)
	out := tgt.run(t, false, katyotiBin, "solution", "install", "demo-rel@v1",
		"--universe", tgt.workspace, "--non-interactive",
		"--set", "LEANIX_API_TOKEN=tok-xyz")

	if !strings.Contains(out, "all keys set") {
		t.Fatalf("bootstrap with a config-set should report the keys collected: %s", out)
	}
	// The member installed...
	if _, err := os.Stat(filepath.Join(tgt.root, "bin", "store-content", "flowpack", "1.0.0")); err != nil {
		t.Fatalf("release member should be installed: %v", err)
	}
	// ...the template landed...
	if _, err := os.Stat(filepath.Join(tgt.workspace, ".katyoti", "config", "LeanIX.yaml")); err != nil {
		t.Fatalf("config-set template should be applied into the universe: %v", err)
	}
	// ...and the key was collected into the universe .env.
	if env := readFileString(t, filepath.Join(tgt.workspace, ".katyoti", ".env")); !strings.Contains(env, "LEANIX_API_TOKEN=tok-xyz") {
		t.Fatalf("bootstrap should have collected the config-set key:\n%s", env)
	}

	// --- a bootstrap that leaves a key unset is REFUSED before anything installs ---
	bare := newInst(t)
	writeFile(t, filepath.Join(bare.workspace, ".katyoti", "config", "Univers.yaml"), "name: bare\n")
	bare.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	bare.run(t, false, katyotiBin, "marketplace", "add", "releases", catalog)
	if out := bare.run(t, true, katyotiBin, "solution", "install", "demo-rel@v1",
		"--universe", bare.workspace, "--non-interactive"); !strings.Contains(out, "LEANIX_API_TOKEN") {
		t.Fatalf("a bootstrap missing a config-set key must fail naming it: %s", out)
	}
	if _, err := os.Stat(filepath.Join(bare.root, "bin", "store-content", "flowpack")); err == nil {
		t.Fatal("a refused bootstrap installed a member")
	}

	// --- the key from a sealed keys file: exported where it is set, imported by
	// the bootstrap into the new universe ---
	t.Setenv("KTW_E2E_KEYS_PASSWORD", "a long enough password")
	keysFile := filepath.Join(tgt.workspace, "northwind.keys")
	tgt.run(t, false, katyotiBin, "keys", "export", keysFile, "--universe", tgt.workspace,
		"--config-set", "northwind-base", "--yes", "--password-env", "KTW_E2E_KEYS_PASSWORD")
	keyed := newInst(t)
	writeFile(t, filepath.Join(keyed.workspace, ".katyoti", "config", "Univers.yaml"), "name: keyed\n")
	keyed.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	keyed.run(t, false, katyotiBin, "marketplace", "add", "releases", catalog)
	out = keyed.run(t, false, katyotiBin, "solution", "install", "demo-rel@v1",
		"--universe", keyed.workspace, "--non-interactive",
		"--keys", keysFile, "--password-env", "KTW_E2E_KEYS_PASSWORD")
	if !strings.Contains(out, "all keys set") {
		t.Fatalf("a bootstrap given the keys file should need nothing more: %s", out)
	}
	if env := readFileString(t, filepath.Join(keyed.workspace, ".katyoti", ".env")); !strings.Contains(env, "LEANIX_API_TOKEN=tok-xyz") {
		t.Fatalf("the keys file's key should be in the new universe's .env:\n%s", env)
	}

	// --- COMPATIBILITY: a release WITHOUT a config-set behaves exactly as before ---
	src.run(t, false, composeBin, "snapshot-universe", src.workspace,
		"--version", "v2", "--marketplace", filepath.Join(market, "katyoti-marketplace.yaml"), "--name", "plain-rel")
	plain := newInst(t)
	writeFile(t, filepath.Join(plain.workspace, ".katyoti", "config", "Univers.yaml"), "name: plain\n")
	plain.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	plain.run(t, false, katyotiBin, "marketplace", "add", "releases", catalog)
	out = plain.run(t, false, katyotiBin, "solution", "install", "plain-rel@v2", "--universe", plain.workspace)
	if strings.Contains(out, "Keys:") {
		t.Errorf("a release without a config-set must not run the key flow: %s", out)
	}
	if _, err := os.Stat(filepath.Join(plain.root, "bin", "store-content", "flowpack", "1.0.0")); err != nil {
		t.Fatalf("a plain release must still install its members: %v", err)
	}
}

// TestKeysExportImport: keys travel as one file sealed with a password,
// holding ONLY the keys a manifest declares — a key added by hand to the same
// .env never leaves — each written back into its scope's .env; a config-set
// apply then needs nothing more. A wrong password opens nothing.
func TestKeysExportImport(t *testing.T) {
	pub := newInst(t)
	market := filepath.Join(pub.workspace, "market")
	src := filepath.Join(pub.workspace, "northwind-base")
	writeFile(t, filepath.Join(src, "keys.yaml"), `apiVersion: 1
keys:
  - name: LEANIX_API_TOKEN
    secret: true
  - name: CONFLUENCE_BASE_URL
    scope: global
`)
	writeFile(t, filepath.Join(src, "config", "LeanIX.yaml"), "token: ${{env:LEANIX_API_TOKEN}}\nbase: ${{env:CONFLUENCE_BASE_URL}}\n")
	pub.run(t, false, composeBin, "build-config-set", src,
		"--name", "northwind-base", "--version", "1.0.0", "--output", market)
	catalog := filepath.Join(market, "katyoti-marketplace.yaml")
	t.Setenv("KTW_E2E_KEYS_PASSWORD", "a long enough password")

	// --- where the keys are set: apply, add a key by hand, export ---
	from := newInst(t)
	from.run(t, false, katyotiBin, "marketplace", "add", "demo", fileURL(catalog))
	from.run(t, false, katyotiBin, "config-set", "apply", "northwind-base",
		"--universe", from.workspace, "--non-interactive",
		"--set", "LEANIX_API_TOKEN=tok-${NOT_EXPANDED}",
		"--set", "CONFLUENCE_BASE_URL=https://confluence.example")
	appendFile(t, filepath.Join(from.workspace, ".katyoti", ".env"), "MY_OWN_KEY=never-shared\n")
	keysFile := filepath.Join(from.workspace, "northwind.keys")
	out := from.run(t, false, katyotiBin, "keys", "export", keysFile, "--universe", from.workspace,
		"--config-set", "northwind-base", "--yes", "--password-env", "KTW_E2E_KEYS_PASSWORD")
	if strings.Contains(out, "MY_OWN_KEY") || !strings.Contains(out, "CONFLUENCE_BASE_URL") {
		t.Fatalf("the export lists the declared keys only: %s", out)
	}
	if b := readFileString(t, keysFile); strings.Contains(b, "tok-") || strings.Contains(b, "never-shared") {
		t.Fatal("the keys file holds a value in clear")
	}
	// No manifest, no export: never a whole .env.
	if out := from.run(t, true, katyotiBin, "keys", "export", keysFile+"2", "--universe", from.workspace,
		"--yes", "--password-env", "KTW_E2E_KEYS_PASSWORD"); !strings.Contains(out, "never a whole .env") {
		t.Fatalf("an export without a manifest must be refused: %s", out)
	}

	// --- another machine: a wrong password opens nothing; the right one
	// writes each key into its scope ---
	to := newInst(t)
	to.run(t, false, katyotiBin, "marketplace", "add", "demo", fileURL(catalog))
	t.Setenv("KTW_E2E_WRONG_PASSWORD", "not the password at all")
	if out := to.run(t, true, katyotiBin, "keys", "import", keysFile, "--universe", to.workspace,
		"--password-env", "KTW_E2E_WRONG_PASSWORD"); !strings.Contains(out, "password") {
		t.Fatalf("a wrong password must be refused: %s", out)
	}
	to.run(t, false, katyotiBin, "keys", "import", keysFile, "--universe", to.workspace,
		"--password-env", "KTW_E2E_KEYS_PASSWORD")
	uniEnv := readFileString(t, filepath.Join(to.workspace, ".katyoti", ".env"))
	if !strings.Contains(uniEnv, "tok-${NOT_EXPANDED}") || strings.Contains(uniEnv, "CONFLUENCE_BASE_URL") || strings.Contains(uniEnv, "MY_OWN_KEY") {
		t.Fatalf("the universe .env should hold the universe key verbatim, nothing else:\n%s", uniEnv)
	}
	if g := readFileString(t, filepath.Join(to.root, ".env")); !strings.Contains(g, "CONFLUENCE_BASE_URL=https://confluence.example") {
		t.Fatalf("the global key should be in the global .env:\n%s", g)
	}
	out = to.run(t, false, katyotiBin, "config-set", "apply", "northwind-base",
		"--universe", to.workspace, "--non-interactive")
	if !strings.Contains(out, "all keys set") {
		t.Fatalf("after the import the apply needs nothing more: %s", out)
	}
}

// --- release with a seed: a universe's content published and bootstrapped ---

// TestBootstrapWithSeed: a release snapshotted with its project carries a seed
// (its objects, documents and files, the universe's tasks — never a secret);
// `bootstrap` into a named universe lays the project out, attaches it and gives
// the universe its tasks; a second bootstrap keeps the project; a seed whose
// bytes changed is refused.
func TestBootstrapWithSeed(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	_ = os.MkdirAll(dist, 0o755)
	archive, sha := contentPack(t, dist, "flowpack")
	contentCat := filepath.Join(dist, "katyoti-marketplace.yaml")
	writeFile(t, contentCat, fmt.Sprintf(`apiVersion: 1
marketplace: {name: content}
items:
  - name: flowpack
    kind: content
    versions: [{version: "1.0.0", assets: [{url: "%s", sha256: "%s"}]}]
`, fileURL(archive), sha))

	src := newInst(t)
	writeFile(t, filepath.Join(src.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\ndisplay_name: Demo\n")
	src.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	src.run(t, false, katyotiBin, "install", "flowpack")
	src.run(t, false, katyotiBin, "universe", "wire", "flowpack")
	project := filepath.Join(t.TempDir(), "demo-src-project")
	writeFile(t, filepath.Join(project, "README.md"), "# The demo\n")
	writeFile(t, filepath.Join(project, "modules", "demo", "objects", "demo.cwm.yaml"), "id: demo\ntype: module\nname: Demo\n")
	writeFile(t, filepath.Join(project, "modules", "demo", "bienvenue.md"), "# Bienvenue\n")
	writeFile(t, filepath.Join(project, ".katyoti", "config", "workspace.yaml"), "universe: demo\n") // the source's attachment: left out
	tasks := filepath.Join(src.workspace, ".katyoti", "tasks")
	writeFile(t, filepath.Join(tasks, "welcome.md"), "---\nname: welcome\ntype: simulator\n---\n")
	releases := filepath.Join(dist, "releases.yaml")

	// A secret in the project refuses the seed, and nothing is published.
	writeFile(t, filepath.Join(project, ".env"), "LEANIX_API_TOKEN=secret\n")
	if out := src.run(t, true, composeBin, "snapshot-universe", src.workspace, "--version", "v1", "--marketplace", releases,
		"--name", "demo-rel", "--project", project, "--tasks", tasks); !strings.Contains(out, "secret") {
		t.Fatalf("a project holding a .env must be refused naming the secret: %s", out)
	}
	if _, err := os.Stat(releases); err == nil && strings.Contains(readFileString(t, releases), "demo-rel") {
		t.Fatal("a refused seed still published its release")
	}
	_ = os.Remove(filepath.Join(project, ".env"))
	src.run(t, false, composeBin, "snapshot-universe", src.workspace, "--version", "v1", "--marketplace", releases,
		"--name", "demo-rel", "--project", project, "--tasks", tasks)
	cat := readFileString(t, releases)
	if !strings.Contains(cat, "kind: seed") || !strings.Contains(cat, "seeds/v1/demo-rel-seed.tar.gz") || !strings.Contains(cat, "name: demo-rel-seed") {
		t.Fatalf("the snapshot should publish its seed as an item and pin it:\n%s", cat)
	}

	// A release bringing a seed needs a named universe.
	tgt := newInst(t)
	tgt.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	tgt.run(t, false, katyotiBin, "marketplace", "add", "releases", releases)
	if out := tgt.run(t, true, katyotiBin, "solution", "install", "demo-rel@v1"); !strings.Contains(out, "name the universe") {
		t.Fatalf("bootstrapping a seed without --universe must be refused: %s", out)
	}

	home := filepath.Join(t.TempDir(), "demo")
	person := t.TempDir()
	tgt = tgt.withHome(person)
	out := tgt.run(t, false, katyotiBin, "solution", "install", "demo-rel@v1", "--universe", home, "--non-interactive")
	// The seed's project, in the projects folder under the universe's name.
	proj := filepath.Join(person, "Documents", "Katyoti", "demo")
	if !strings.Contains(out, proj) || !strings.Contains(out, "laid out and attached") {
		t.Fatalf("bootstrap should report the project it laid out: %s", out)
	}
	for _, rel := range []string{"README.md", "modules/demo/objects/demo.cwm.yaml", "modules/demo/bienvenue.md"} {
		if _, err := os.Stat(filepath.Join(proj, filepath.FromSlash(rel))); err != nil {
			t.Errorf("the seed's %s is not in the project: %v", rel, err)
		}
	}
	if ws := readFileString(t, filepath.Join(proj, ".katyoti", "project.yaml")); !strings.Contains(ws, "universe: demo") {
		t.Errorf("the project is not attached to the new universe:\n%s", ws)
	}
	if _, err := os.Stat(filepath.Join(proj, ".katyoti", "config", "workspace.yaml")); err == nil {
		t.Error("the source project's attachment travelled with the seed")
	}
	if _, err := os.Stat(filepath.Join(home, ".katyoti", "tasks", "welcome.md")); err != nil {
		t.Errorf("the universe did not get the seed's tasks: %v", err)
	}

	// A second bootstrap keeps the person's project.
	writeFile(t, filepath.Join(proj, "README.md"), "# Mine now\n")
	out = tgt.run(t, false, katyotiBin, "solution", "install", "demo-rel@v1", "--universe", home, "--non-interactive")
	if !strings.Contains(out, "kept as it was") || readFileString(t, filepath.Join(proj, "README.md")) != "# Mine now\n" {
		t.Fatalf("a second bootstrap must keep the project: %s", out)
	}

	// A seed whose bytes changed is refused.
	seedFile := filepath.Join(dist, "seeds", "v1", "demo-rel-seed.tar.gz")
	data, _ := os.ReadFile(seedFile)
	_ = os.WriteFile(seedFile, append(data, 0), 0o644)
	other := newInst(t).withHome(t.TempDir())
	other.run(t, false, katyotiBin, "marketplace", "add", "content", contentCat)
	other.run(t, false, katyotiBin, "marketplace", "add", "releases", releases)
	if out := other.run(t, true, katyotiBin, "solution", "install", "demo-rel@v1", "--universe", filepath.Join(t.TempDir(), "demo"), "--non-interactive"); !strings.Contains(out, "sha256") {
		t.Fatalf("a changed seed must be refused on its hash: %s", out)
	}
}

// TestSeedApply: a seed is an item of its own — built alone, applied to a
// universe like a config-set. The universe must have the members it was made
// with; each application lays out a NEW project with its OWN identity (the
// seed's project object re-minted); a second project joins a universe beside
// the first, and one the attach's member rules refuse is taken back; a folder
// holding files is refused.
func TestSeedApply(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	_ = os.MkdirAll(dist, 0o755)
	archive, sha := contentPack(t, dist, "flowpack")
	catalog := filepath.Join(dist, "katyoti-marketplace.yaml")
	writeFile(t, catalog, fmt.Sprintf(`apiVersion: 1
marketplace: {name: demo}
items:
  - name: flowpack
    kind: content
    versions: [{version: "1.0.0", assets: [{url: "%s", sha256: "%s"}]}]
`, fileURL(archive), sha))
	pub := newInst(t)
	project := filepath.Join(t.TempDir(), "source-project")
	writeFile(t, filepath.Join(project, "project.cwm.yaml"), "id: source-project\ntype: project\nname: The demo\ncapabilities:\n  project:\n    uuid: 01999999-aaaa-7bbb-8ccc-dddddddddddd\n")
	writeFile(t, filepath.Join(project, "modules", "demo", "objects", "demo.cwm.yaml"), "id: demo\ntype: module\nname: Demo\n")
	writeFile(t, filepath.Join(project, "docs", "notes.txt"), "a file of the project, not an object\n")
	pub.run(t, false, composeBin, "build-seed", project, "--name", "demo-seed", "--version", "1.0.0",
		"--output", dist, "--requires", "flowpack@1.0.0")
	if cat := readFileString(t, catalog); !strings.Contains(cat, "kind: seed") || !strings.Contains(cat, "members:") {
		t.Fatalf("build-seed should publish a seed item with its members:\n%s", cat)
	}

	tgt := newInst(t)
	tgt.run(t, false, katyotiBin, "marketplace", "add", "demo", catalog)
	if out := tgt.run(t, false, katyotiBin, "seed", "list"); !strings.Contains(out, "demo-seed") {
		t.Fatalf("seed list should offer the seed: %s", out)
	}

	// A universe without the members the seed was made with is refused.
	bare := filepath.Join(t.TempDir(), "bare")
	writeFile(t, filepath.Join(bare, ".katyoti", "config", "Univers.yaml"), "name: bare\n")
	if out := tgt.run(t, true, katyotiBin, "seed", "apply", "demo-seed", "--universe", bare); !strings.Contains(out, "flowpack") {
		t.Fatalf("a universe missing flowpack must be refused naming it: %s", out)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(bare), "bare-project")); err == nil {
		t.Fatal("a refused seed still laid its project out")
	}

	// Two universes with them: two applications, two projects, two identities.
	uuids := map[string]bool{}
	var first inst
	for k, folder := range []string{"first-project", "second-project"} {
		u := tgt
		if k == 1 {
			u = newInst(t)
			u.run(t, false, katyotiBin, "marketplace", "add", "demo", catalog)
		} else {
			first = u
		}
		writeFile(t, filepath.Join(u.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\n")
		u.run(t, false, katyotiBin, "install", "flowpack")
		u.run(t, false, katyotiBin, "universe", "wire", "flowpack")
		dir := filepath.Join(t.TempDir(), folder)
		out := u.run(t, false, katyotiBin, "seed", "apply", "demo-seed", "--universe", u.workspace, "--project", dir)
		if !strings.Contains(out, "laid out and attached") {
			t.Fatalf("seed apply should report the project: %s", out)
		}
		for _, rel := range []string{"modules/demo/objects/demo.cwm.yaml", "docs/notes.txt"} {
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
				t.Errorf("the seed's %s is not in the project: %v", rel, err)
			}
		}
		obj := readFileString(t, filepath.Join(dir, "project.cwm.yaml"))
		if !strings.Contains(obj, "id: "+folder) || strings.Contains(obj, "01999999-aaaa") || !strings.Contains(obj, "name: The demo") {
			t.Errorf("the project object was not re-minted for %s:\n%s", folder, obj)
		}
		uuids[obj[strings.Index(obj, "uuid: ")+6:][:36]] = true
		if ws := readFileString(t, filepath.Join(dir, ".katyoti", "project.yaml")); !strings.Contains(ws, "universe:") {
			t.Errorf("%s is not attached: %s", folder, ws)
		}
	}
	if len(uuids) != 2 {
		t.Error("two projects applied from one seed share a uuid")
	}

	// A universe serving one project takes a second beside it:
	// laid out, recorded as created here, attached - both members listed.
	third := filepath.Join(t.TempDir(), "third-project")
	if out := first.run(t, false, katyotiBin, "seed", "apply", "demo-seed", "--universe", first.workspace, "--project", third); !strings.Contains(out, "laid out and attached") {
		t.Fatalf("a universe already serving a project must take a second beside it: %s", out)
	}
	members := readFileString(t, filepath.Join(first.workspace, ".katyoti", "projects.yaml"))
	if !strings.Contains(members, "first-project") || !strings.Contains(members, "third-project") {
		t.Errorf("both projects should be members of the universe:\n%s", members)
	}

	// One the member rules refuse - a second project of the key first-project
	// - is taken back: nothing laid out stays, the refusal says why.
	twin := filepath.Join(t.TempDir(), "first-project")
	if out := first.run(t, true, katyotiBin, "seed", "apply", "demo-seed", "--universe", first.workspace, "--project", twin); !strings.Contains(out, "does not join") || !strings.Contains(out, "one project served twice") {
		t.Fatalf("a project the member rules refuse must be refused naming the rule: %s", out)
	}
	if _, err := os.Stat(twin); err == nil {
		t.Error("a refused seed still left its project laid out")
	}
	// Its creation record is taken back too: the record never names a place
	// a later clone could fill.
	if createdRecordLists(t, first, twin) {
		t.Errorf("a refused seed's folder is still recorded as created on this PC:\n%s", createdRecord(t, first))
	}

	// A folder that already holds files is refused.
	mine := filepath.Join(t.TempDir(), "mine")
	writeFile(t, filepath.Join(mine, "README.md"), "mine\n")
	fresh := newInst(t)
	fresh.run(t, false, katyotiBin, "marketplace", "add", "demo", catalog)
	writeFile(t, filepath.Join(fresh.workspace, ".katyoti", "config", "Univers.yaml"), "name: demo\n")
	fresh.run(t, false, katyotiBin, "install", "flowpack")
	fresh.run(t, false, katyotiBin, "universe", "wire", "flowpack")
	if out := fresh.run(t, true, katyotiBin, "seed", "apply", "demo-seed", "--universe", fresh.workspace, "--project", mine); !strings.Contains(out, "already holds files") {
		t.Fatalf("a project folder holding files must be refused: %s", out)
	}
}
