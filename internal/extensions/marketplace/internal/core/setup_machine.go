// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/catalog"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/infra"
)

// SetupMachine is first run, in one call.
//
// The whole point is that a new user runs ONE instruction and ends up in the
// GUI. Every step below is something they would otherwise have to know to do,
// in the right order, from a machine that has nothing:
//
//  1. install the control plane from the channel they bootstrapped from
//  2. REMEMBER that channel (with its trust), so later updates resolve without
//     the env var the installer script set for one process
//  3. subscribe the marketplace published beside it, PINNED to the same key --
//     otherwise a freshly installed machine can run nothing and the user has to
//     paste a URL they were never shown
//
// Launching the GUI is deliberately NOT here: this package has no business
// spawning a window. The CLI does it after this returns, which also keeps the
// step skippable (`--no-gui`) for CI without threading a flag through a service.
//
// Idempotent. Re-running on a configured machine re-installs nothing and
// re-subscribes nothing; it is safe as a repair command.
func (s *infraOps) SetupMachine(ctx context.Context) (mktapi.SetupMachineOutput, error) {
	var out mktapi.SetupMachineOutput

	ch, err := s.resolveInfraChannel()
	if err != nil {
		return out, err
	}
	out.Channel = ch.Root

	infraOut, err := s.InstallInfra(ctx)
	if err != nil {
		return out, err
	}
	for _, ins := range infraOut.Installed {
		out.Installed = append(out.Installed, ins.Name)
		out.Version = ins.Version
	}
	out.Skipped = infraOut.Skipped

	// The GUI's runtime (WebView2 on Windows), before anything opens the GUI:
	// a clean machine would otherwise open on its download window. Never a
	// failed setup: the runtime is said missing and the rest goes on.
	if done, err := ensureGUIRuntime(ctx, thisGUIRuntime()); err != nil {
		out.GUIRuntimeError = err.Error()
	} else {
		out.GUIRuntime = done
	}

	// TRUST ON FIRST USE. The installer points at a channel via an env var,
	// which carries no trust, so without this the machine would install once and
	// then never verify a signature again -- with the publisher's key sitting
	// right there in the same tree.
	//
	// Adopting it here is exactly as strong as the decision the user already
	// made by running that installer, and strictly stronger afterwards: every
	// LATER fetch is checked against the key seen at install time, so a tree
	// compromised after the fact is caught.
	if ch.Trust != mktapi.TrustPinned {
		if pub, err := ch.SiblingPubKey(ctx); err == nil {
			ch.Trust, ch.PubKey = mktapi.TrustPinned, pub
			out.Pinned = true
		}
	}

	// Persist the channel so the NEXT run resolves it without $KTW_INFRA_CHANNEL,
	// which the installer script only set for its own process.
	if err := infra.Save(ch); err != nil {
		return out, fmt.Errorf("record the infra channel: %w", err)
	}

	// Subscribe the catalog beside the channel, pinned to the same key. A
	// machine that just installed the control plane but knows no marketplace
	// can search nothing and install nothing -- the bootstrap would be
	// technically complete and practically useless.
	if name, err := s.subscribeSibling(ctx, ch); err != nil {
		// NON-FATAL. The control plane is installed and working; a missing
		// catalog is a smaller problem than failing a bootstrap that mostly
		// succeeded, and `marketplace add` fixes it by hand.
		out.MarketplaceError = err.Error()
	} else {
		out.Marketplace = name
	}
	// The solution the channel names: read after the key is pinned, so the
	// pointer that names it is verified.
	if sol, err := ch.Solution(ctx); err == nil {
		out.Solution = sol
	}
	return out, nil
}

// subscribeSibling subscribes <deploy-root>/katyoti-marketplace.yaml, pinned to
// the key published in the same root. Returns the subscription name, or the
// existing one when the machine is already subscribed.
func (s *service) subscribeSibling(ctx context.Context, ch infra.Channel) (string, error) {
	url := ch.SiblingCatalogURL()
	if url == "" {
		return "", fmt.Errorf("cannot derive a catalog URL from channel %s", ch.Root)
	}
	existing, err := s.reg.List()
	if err != nil {
		return "", err
	}
	for _, m := range existing {
		if m.URL == url {
			return m.Name, nil // already subscribed -- idempotent
		}
	}
	in := mktapi.AddMarketplaceInput{Name: "katyoti", URL: url}
	// Pin when the root publishes a key. Without one we still subscribe, but
	// unpinned -- and say so via Trust, rather than pretending it is verified.
	if pub, err := ch.SiblingPubKey(ctx); err == nil {
		in.PubKey = pub
		in.Trust = mktapi.TrustPinned
	}
	// Subscribed under the marketplace's OWN name (a team's marketplace is
	// not « katyoti »), unless another subscription already holds that name.
	if cat, err := catalog.Fetch(ctx, mktapi.Marketplace{Name: in.Name, URL: url, Trust: in.Trust, PubKey: in.PubKey}); err == nil {
		if name := strings.TrimSpace(cat.Marketplace.Name); name != "" && !strings.ContainsAny(name, `/\ `) {
			in.Name = name
		}
	}
	for i, base := 2, in.Name; ; i++ {
		taken := false
		for _, m := range existing {
			taken = taken || m.Name == in.Name
		}
		if !taken {
			break
		}
		in.Name = fmt.Sprintf("%s-%d", base, i)
	}
	added, err := s.AddMarketplace(ctx, in)
	if err != nil {
		return "", err
	}
	return added.Name, nil
}
