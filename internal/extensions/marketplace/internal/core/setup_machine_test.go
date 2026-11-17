// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/infra"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/registry"
)

// seedDeployRoot writes a deploy root exactly as `make release` produces one:
// a catalog, a published key, and an infra channel beside them. Returns the
// channel path.
func seedDeployRoot(t *testing.T, version string) string {
	t.Helper()
	root := t.TempDir()
	channel := filepath.Join(root, "infra")
	plat := filepath.Join(channel, version, infra.Platform())
	if err := os.MkdirAll(plat, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(p, body string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(channel, infra.PointerFile), "version: "+version+"\n")
	write(filepath.Join(root, "katyoti-marketplace.yaml"),
		"apiVersion: 1\nmarketplace:\n    name: test\nitems: []\n")
	write(filepath.Join(root, "katyoti.key.pub"), "untrusted comment: test\nRWTestKeyNotReal\n")

	// One real binary + its checksum, so the install path is exercised rather
	// than short-circuited.
	exe := "katyoti"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	write(filepath.Join(plat, exe), "not-a-real-binary")
	write(filepath.Join(plat, infra.SumsFile), sha256Of(t, filepath.Join(plat, exe))+"  "+exe+"\n")
	return channel
}

func sha256Of(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// SetupMachine is the ONE path a new user takes, so the invariant that matters
// is not "it installed something" — it is that the machine is left USABLE.
//
// A machine can finish install-infra perfectly and still search nothing and
// install nothing, because it knows no catalog. That state is technically
// complete and practically useless, and nobody would report it as a bug: they
// would just conclude the tool does not work.
func TestSetupMachine_LeavesTheMachineUsable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	channel := seedDeployRoot(t, "v1.0.0")
	t.Setenv(infra.EnvChannel, channel)
	application.ResetGlobals() // the root above, not a cached one
	t.Cleanup(application.ResetGlobals)

	reg, err := registry.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &infraOps{&service{reg: reg}}

	out, err := s.SetupMachine(context.Background())
	if err != nil {
		t.Fatalf("SetupMachine: %v", err)
	}

	// 1. The catalog beside the channel must be subscribed, or the user has to
	//    paste a URL they were never shown.
	if out.Marketplace == "" {
		t.Errorf("no marketplace subscribed (%s) — the machine can install nothing", out.MarketplaceError)
	}
	ms, err := reg.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) == 0 {
		t.Fatal("the subscription was reported but not recorded")
	}

	// 2. The channel must be REMEMBERED. The installer sets it via an env var
	//    for one process; without persisting it, the very next command on this
	//    machine cannot find the control plane it just installed.
	recorded := filepath.Join(root, "infra-channel.yaml")
	body, err := os.ReadFile(recorded)
	if err != nil {
		t.Fatalf("the infra channel was not recorded: %v", err)
	}
	if !strings.Contains(string(body), "root:") {
		t.Errorf("recorded channel has no root: %s", body)
	}
}

// Re-running setup on a configured machine must not subscribe the same
// marketplace twice. It is the documented repair command, so the second run is
// the NORMAL case, not an edge case.
func TestSetupMachine_IsIdempotent(t *testing.T) {
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	t.Setenv(infra.EnvChannel, seedDeployRoot(t, "v1.0.0"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	reg, err := registry.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &infraOps{&service{reg: reg}}
	for i := 0; i < 2; i++ {
		if _, err := s.SetupMachine(context.Background()); err != nil {
			t.Fatalf("SetupMachine run %d: %v", i+1, err)
		}
	}
	ms, err := reg.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 {
		t.Errorf("after two runs there are %d subscriptions, want 1", len(ms))
	}
}
