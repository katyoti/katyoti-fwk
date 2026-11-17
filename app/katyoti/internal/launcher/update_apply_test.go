// SPDX-License-Identifier: Apache-2.0

package launcher

import (
	"bytes"
	"context"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// fakeInfra stands for the installation's control plane: its versions, and the
// move a self-update makes (no file of any installation is touched).
type fakeInfra struct {
	versions mktapi.InfraVersionsOutput
	done     []mktapi.Update
	updated  bool
}

func (f *fakeInfra) InfraVersions(context.Context) (mktapi.InfraVersionsOutput, error) {
	return f.versions, nil
}

func (f *fakeInfra) SelfUpdate(context.Context) ([]mktapi.Update, error) {
	f.updated = true
	return f.done, nil
}

// katyoti gui --update leaves a control plane make dev placed, as update --all
// does, and says so; a channel's control plane is updated.
func TestGuiUpdateLeavesADevControlPlane(t *testing.T) {
	dev := &fakeInfra{
		versions: mktapi.InfraVersionsOutput{Installed: "c4ea4719-dirty", Current: "v2026.10-rc7", DevBuild: true},
		done:     []mktapi.Update{{Name: "katyoti", Installed: "c4ea4719-dirty", Latest: "v2026.10-rc7"}},
	}
	var w bytes.Buffer
	if !updateControlPlane(context.Background(), &w, dev, dev) {
		t.Errorf("a dev control plane left says nothing")
	}
	if dev.updated {
		t.Errorf("katyoti gui --update replaced the dev control plane")
	}
	if got := w.String(); got != "update: dev build c4ea4719-dirty (make dev) left as it is - `katyoti update --infra` replaces it with the infra channel's set\n" {
		t.Errorf("dev control plane: %q", got)
	}

	channel := &fakeInfra{
		versions: mktapi.InfraVersionsOutput{Installed: "v1", Current: "v2"},
		done:     []mktapi.Update{{Name: "katyoti", Installed: "v1", Latest: "v2"}},
	}
	w.Reset()
	if !updateControlPlane(context.Background(), &w, channel, channel) || !channel.updated {
		t.Errorf("a channel's control plane is not updated")
	}
	if got := w.String(); got != "updated katyoti v1 -> v2\n" {
		t.Errorf("channel control plane: %q", got)
	}
}
