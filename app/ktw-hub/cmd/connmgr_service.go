// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// connMgrServiceImpl is the bus-exposed control surface that wraps the
// hub's in-process ConnectionManager. EnsureSpawned is the only useful
// bus operation here — Acquire's return value (a Sender) isn't
// serializable, so we offer "spawn it for me; I'll address its slot
// myself" as the spawn trigger.
type connMgrServiceImpl struct {
	app *application.Application
}

func newConnMgrService(ctx context.Context) (cmapi.ConnMgrService, error) {
	app := application.AppFromContext(ctx)
	if app == nil {
		return nil, fmt.Errorf("conn-mgr service factory: no Application in ctx")
	}
	return &connMgrServiceImpl{app: app}, nil
}

// EnsureSpawned drives the spawn lifecycle: Acquire bumps the refcount
// (spawns if not already running, waits for ktsp/ping), then Release
// drops it back. After this returns successfully, the named sidecar is
// guaranteed to have subscribed to its slot subjects, so a re-issued
// bus call from the original caller will reach it.
//
// Idempotent: an already-running sidecar gets a brief refcount blip,
// returning quickly.
func (s *connMgrServiceImpl) EnsureSpawned(ctx context.Context, in cmapi.EnsureSpawnedInput) (cmapi.EnsureSpawnedOutput, error) {
	if in.Name == "" {
		return cmapi.EnsureSpawnedOutput{}, ktsp.ErrInvalidRequest("conn-mgr/ensure-spawned: name is required")
	}
	mgr, err := application.AppUseService[cmapi.ConnectionManager](s.app)
	if err != nil {
		return cmapi.EnsureSpawnedOutput{}, fmt.Errorf("conn-mgr: resolve ConnectionManager: %w", err)
	}
	if _, err := mgr.Acquire(ctx, in.Name); err != nil {
		if errors.Is(err, cmapi.ErrSidecarNotInUniverse) {
			return cmapi.EnsureSpawnedOutput{}, ktsp.ErrSlotNotFound(in.Name)
		}
		return cmapi.EnsureSpawnedOutput{}, fmt.Errorf("conn-mgr: acquire %s: %w", in.Name, err)
	}
	// Release immediately — the caller didn't ask us to hold the
	// connection; it only asked us to trigger the spawn. Idle-reaper
	// keeps the sidecar alive as long as bus traffic flows; otherwise
	// it'll drain naturally per the SpawnSpec.IdleTimeout.
	mgr.Release(in.Name)
	return cmapi.EnsureSpawnedOutput{}, nil
}

// Members answers every member's state from the hub's ConnectionManager.
func (s *connMgrServiceImpl) Members(ctx context.Context, _ cmapi.MembersInput) (cmapi.MembersOutput, error) {
	mgr, err := application.AppUseService[cmapi.ConnectionManager](s.app)
	if err != nil {
		return cmapi.MembersOutput{}, fmt.Errorf("conn-mgr: resolve ConnectionManager: %w", err)
	}
	return cmapi.MembersOutput{Members: mgr.All()}, nil
}

// Restart stops a running member and starts it again (: keys
// saved while it ran); a member not running is left alone.
func (s *connMgrServiceImpl) Restart(ctx context.Context, in cmapi.RestartInput) (cmapi.RestartOutput, error) {
	if in.Name == "" {
		return cmapi.RestartOutput{}, ktsp.ErrInvalidRequest("conn-mgr/restart: name is required")
	}
	mgr, err := application.AppUseService[cmapi.ConnectionManager](s.app)
	if err != nil {
		return cmapi.RestartOutput{}, fmt.Errorf("conn-mgr: resolve ConnectionManager: %w", err)
	}
	restarted, err := mgr.Restart(ctx, in.Name)
	if errors.Is(err, cmapi.ErrSidecarNotInUniverse) {
		return cmapi.RestartOutput{}, ktsp.ErrSlotNotFound(in.Name)
	}
	if err != nil {
		return cmapi.RestartOutput{}, fmt.Errorf("conn-mgr: restart %s: %w", in.Name, err)
	}
	return cmapi.RestartOutput{Restarted: restarted}, nil
}

// connMgrServiceExtension publishes ConnMgrService as a KTSP slot on
// the hub. Subject form: `ktw-hub:/conn-mgr/<method>` — clients
// (messagebus.Sender's retry wrapper) call it on the cold path of
// the A-RETRY lazy-spawn flow.
//
// Lives in ktw-hub/cmd/ rather than connmgr/local/ because the
// implementation pulls a ConnectionManager service from the hub's
// application container — the connmgr/local extension doesn't know
// (and shouldn't know) about the hub binary.
func connMgrServiceExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishSimpleService[cmapi.ConnMgrService](
			newConnMgrService,
			appcore.WithServiceDef(cmapi.ConnMgrServiceDef),
			appcore.WithKTSPContainer(),
			// CLI not generated — operators use `ktw-hub connections`
			// (the existing connmgr Cobra group); this is a wire
			// contract for the sender retry wrapper.
			appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
		))
	}
}
