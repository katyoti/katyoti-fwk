// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
)

// Doctor runs the full integrity check of the scheduler's SQLite store - the
// file in the environment's SchedulerDir(), opened read-only beside the store
// in use, so a store an open refused as damaged is examined too - and names
// each fault with its repair. An open runs only the quick check.
func (s *Service) Doctor(ctx context.Context, _ api.DoctorInput) (api.DoctorOutput, error) {
	env, _ := application.UseService[application.AppEnvironment](ctx)
	dir := ""
	if env != nil {
		dir = env.SchedulerDir()
	}
	if dir == "" {
		return api.DoctorOutput{}, fmt.Errorf("%w: the scheduler's store: the environment names no scheduler folder (SchedulerDir)", cwmapi.ErrInvalidInput)
	}
	return doctorOf(ctx, dir)
}

// doctorOf is the full check of the store of dir.
func doctorOf(ctx context.Context, dir string) (api.DoctorOutput, error) {
	path, found, faults, err := sqlstore.Doctor(ctx, dir)
	if err != nil {
		return api.DoctorOutput{}, fmt.Errorf("the scheduler's store %s: %w", path, err)
	}
	out := api.DoctorOutput{Store: path, Found: found, OK: len(faults) == 0, Failed: len(faults), Faults: []api.DoctorFault{}}
	for _, f := range faults {
		out.Faults = append(out.Faults, api.DoctorFault{Fault: f.Fault, Repair: f.Repair})
	}
	return out, nil
}
