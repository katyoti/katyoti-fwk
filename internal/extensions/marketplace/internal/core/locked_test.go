// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// : a locked installation never updates its control plane
// sidecars or marketplace items by itself - each updater says locked, and
// moves nothing.
func TestALockedInstallationRefusesEveryUpdate(t *testing.T) {
	was := installationLock
	installationLock = func() (string, bool) { return "workshop", true }
	t.Cleanup(func() { installationLock = was })

	ctx := context.Background()
	s := &service{}
	infra := &infraOps{s}
	_, errUpdate := s.Update(ctx, mktapi.UpdateInput{})
	_, errSelf := infra.SelfUpdate(ctx)
	_, errSet := infra.SetInfraVersion(ctx, "2026.11.0")
	for name, err := range map[string]error{"sidecar and content update": errUpdate, "control-plane update": errSelf, "control-plane move": errSet} {
		if !errors.Is(err, mktapi.ErrLocked) {
			t.Errorf("%s on a locked installation = %v, want ErrLocked", name, err)
			continue
		}
		if !strings.Contains(err.Error(), "workshop is locked") || !strings.Contains(err.Error(), "release is installed") {
			t.Errorf("%s: %q does not say the installation is locked and how a version moves", name, err)
		}
	}
}

func TestAnUnlockedInstallationIsNotRefused(t *testing.T) {
	was := installationLock
	installationLock = func() (string, bool) { return "", false }
	t.Cleanup(func() { installationLock = was })
	if err := refuseWhenLocked("update"); err != nil {
		t.Fatalf("an unlocked installation refused: %v", err)
	}
}
