// SPDX-License-Identifier: Apache-2.0

package infra

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/signing"
)

// seedChannel writes a minimal but REAL channel: a version pointer plus one
// platform folder with a SHA256SUMS. Returns the channel root.
func seedChannel(t *testing.T, version string, sums string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "infra")
	dir := filepath.Join(root, version, Platform())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, PointerFile), []byte("version: "+version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if sums != "" {
		if err := os.WriteFile(filepath.Join(dir, SumsFile), []byte(sums), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestResolve_EnvWins(t *testing.T) {
	t.Setenv(EnvChannel, "https://example.test/infra/")
	ch, err := Resolve([]Source{{URL: "https://elsewhere.test/katyoti-marketplace.yaml"}})
	if err != nil {
		t.Fatal(err)
	}
	// The trailing slash must be normalised away, or every joined path doubles it.
	if ch.Root != "https://example.test/infra" {
		t.Errorf("Root = %q, want the env value without its trailing slash", ch.Root)
	}
}

// Deriving the channel from the subscribed marketplace is what makes the common
// case need no configuration at all: publish infra as a subfolder, and it is
// simply there.
func TestResolve_DerivesFromMarketplace(t *testing.T) {
	t.Setenv(EnvChannel, "")
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir()) // no infra-channel.yaml record
	ch, err := Resolve([]Source{{URL: "https://example.test/katyoti/katyoti-marketplace.yaml"}})
	if err != nil {
		t.Fatal(err)
	}
	if ch.Root != "https://example.test/katyoti/infra" {
		t.Errorf("Root = %q, want the marketplace folder + /infra", ch.Root)
	}
}

// Two subscriptions implying two different control planes is NOT a coin flip:
// picking wrong means installing someone else's katyoti.exe, which is the
// binary that installs everything else.
func TestResolve_RefusesAmbiguity(t *testing.T) {
	t.Setenv(EnvChannel, "")
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	_, err := Resolve([]Source{
		{URL: "https://a.test/katyoti-marketplace.yaml"},
		{URL: "https://b.test/katyoti-marketplace.yaml"},
	})
	if err == nil {
		t.Fatal("Resolve picked one of two conflicting channels; it must refuse")
	}
	if !strings.Contains(err.Error(), "different infra channels") {
		t.Errorf("error %q does not explain the ambiguity", err)
	}
}

func TestLatestVersion(t *testing.T) {
	t.Setenv(EnvChannel, seedChannel(t, "v9.9.9", ""))
	ch, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ch.LatestVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "v9.9.9" {
		t.Errorf("LatestVersion = %q, want v9.9.9", got)
	}
}

func TestAssets_ParsesSumsAndBuildsURLs(t *testing.T) {
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	sums := "aaa  katyoti" + exe + "\nbbb  ktw-hub" + exe + "\n"
	root := seedChannel(t, "v1.0.0", sums)
	t.Setenv(EnvChannel, root)

	ch, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	assets, err := ch.Assets(context.Background(), "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	// Keys are the catalog-free binary names, so they match
	// controlplane.InfraBinaries without the caller stripping anything.
	if _, ok := assets["katyoti"]; !ok {
		t.Fatalf("no asset keyed \"katyoti\"; got %v", assets)
	}
	if assets["katyoti"].Sha256 != "aaa" {
		t.Errorf("sha256 = %q, want aaa", assets["katyoti"].Sha256)
	}
	want := filepath.Join(root, "v1.0.0", Platform(), "katyoti"+exe)
	if assets["katyoti"].URL != want {
		t.Errorf("URL = %q, want %q", assets["katyoti"].URL, want)
	}
	// A binary the channel does not ship is simply absent — the CALLER owns the
	// required set and reports what is missing.
	if _, ok := assets["ktw-gui"]; ok {
		t.Error("ktw-gui was invented from a SHA256SUMS that never listed it")
	}
}

// A missing platform folder must say WHICH platform, not just "not found":
// "no darwin-arm64 build" is actionable, ENOENT is not.
func TestAssets_MissingPlatformIsExplained(t *testing.T) {
	t.Setenv(EnvChannel, seedChannel(t, "v1.0.0", ""))
	ch, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ch.Assets(context.Background(), "v2.0.0")
	if err == nil {
		t.Fatal("expected an error for a version the channel does not carry")
	}
	if !strings.Contains(err.Error(), Platform()) {
		t.Errorf("error %q does not name the platform", err)
	}
}

// The channel retains exactly two versions and names the rollback target
// EXPLICITLY. Inferring order from folder mtimes would be wrong on a synced
// share (OneDrive, SharePoint rewrite them), and a rollback that picks the
// wrong target reinstalls the very version being backed out of.
func TestVersions_ReadsRollbackTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "infra")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, PointerFile),
		[]byte("version: v2.0.0\nprevious: v1.9.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvChannel, root)
	ch, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	latest, previous, err := ch.Versions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if latest != "v2.0.0" || previous != "v1.9.0" {
		t.Errorf("Versions() = (%q, %q), want (v2.0.0, v1.9.0)", latest, previous)
	}
}

// A channel that has only ever published once has no rollback target, and that
// is not an error -- it just means there is nowhere to step back to yet.
func TestVersions_FirstPublishHasNoPrevious(t *testing.T) {
	t.Setenv(EnvChannel, seedChannel(t, "v1.0.0", ""))
	ch, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	latest, previous, err := ch.Versions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if latest != "v1.0.0" || previous != "" {
		t.Errorf("Versions() = (%q, %q), want (v1.0.0, empty)", latest, previous)
	}
}

// A pinned channel must REFUSE a tampered pointer.
//
// This gate was lost when infra moved off the catalog: the catalog verified a
// pinned signature before any install, and the channel initially checked only
// sha256 -- which whoever can replace a file can replace too. Infra is the one
// binary set where a swap compromises every LATER install, not just one item.
func TestVerify_PinnedChannelRefusesUnsignedPointer(t *testing.T) {
	root := filepath.Join(t.TempDir(), "infra")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, PointerFile), []byte("version: v1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pk, _, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	ch := Channel{Root: root, Source: "test", Trust: mktapi.TrustPinned, PubKey: signing.EncodePublicKey(pk, "test")}

	// No .minisig beside it at all.
	if _, _, err := ch.Versions(context.Background()); err == nil {
		t.Fatal("a pinned channel accepted an UNSIGNED pointer")
	}

	// Now sign it with a DIFFERENT key: present, well-formed, and wrong.
	_, otherSK, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, PointerFile))
	if err != nil {
		t.Fatal(err)
	}
	sig := signing.Sign(body, otherSK, "forged")
	if err := os.WriteFile(filepath.Join(root, PointerFile+".minisig"), []byte(sig), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = ch.Versions(context.Background())
	if err == nil {
		t.Fatal("a pinned channel accepted a pointer signed by the WRONG key")
	}
	if !strings.Contains(err.Error(), "did NOT verify") {
		t.Errorf("error %q does not say the signature failed", err)
	}
}

// An unpinned channel keeps working unsigned -- opt-in trust, matching how an
// unpinned marketplace subscription behaves. Silently enforcing would break
// every existing local/dev channel.
func TestVerify_UnpinnedChannelSkipsTheGate(t *testing.T) {
	t.Setenv(EnvChannel, seedChannel(t, "v1.0.0", ""))
	ch, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ch.Versions(context.Background()); err != nil {
		t.Fatalf("unpinned channel refused an unsigned pointer: %v", err)
	}
}

// The channel names the solution a first install opens on (`solution:`); an
// older channel's `demo:` is still read; the new name wins when both are set.
func TestSolution_ReadsTheNewAndTheOldName(t *testing.T) {
	for _, c := range []struct{ pointer, want string }{
		{"version: v1\nsolution: archi@1.2\n", "archi@1.2"},
		{"version: v1\ndemo: acme-demo@1.004\n", "acme-demo@1.004"},
		{"version: v1\nsolution: archi@1.2\ndemo: old@1\n", "archi@1.2"},
		{"version: v1\n", ""},
	} {
		root := filepath.Join(t.TempDir(), "infra")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, PointerFile), []byte(c.pointer), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvChannel, root)
		ch, err := Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ch.Solution(context.Background())
		if err != nil || got != c.want {
			t.Errorf("Solution() of %q = %q, %v; want %q", c.pointer, got, err, c.want)
		}
	}
}
