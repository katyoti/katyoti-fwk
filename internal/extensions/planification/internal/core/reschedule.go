// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Reschedule moves a release's planned date or its budget — only what is
// given changes — while it has not shipped: a release released or dropped
// keeps the date and budget it had. The change is an update of the release,
// in its history like any other.
func (r *Releases) Reschedule(ctx context.Context, in api.RescheduleInput) (api.ReleaseOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.ReleaseOutput{}, err
	}
	if in.Date == "" && in.BudgetUSD == nil && in.Level == "" {
		return api.ReleaseOutput{}, invalid("Reschedule: give a date, a budget or a level")
	}
	if err := checkLevel("Reschedule", in.Level); err != nil {
		return api.ReleaseOutput{}, err
	}
	if in.Date != "" {
		if _, err := time.Parse("2006-01-02", in.Date); err != nil {
			return api.ReleaseOutput{}, invalid("Reschedule: date %q is not YYYY-MM-DD", in.Date)
		}
	}
	if in.BudgetUSD != nil && *in.BudgetUSD < 0 {
		return api.ReleaseOutput{}, invalid("Reschedule: budget_usd is not negative")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	o, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	if st := lifeState(o); st == api.ReleaseReleased || st == api.ReleaseDropped {
		return api.ReleaseOutput{}, invalid("Reschedule: %s is %s", in.ID, st)
	}
	f := releaseFields(o)
	if in.Date != "" {
		f.Date = in.Date
	}
	if in.BudgetUSD != nil {
		f.BudgetUSD = *in.BudgetUSD
	}
	if in.Level != "" {
		f.Level = in.Level
	}
	out, err := setFields(ctx, cwm, o, f)
	if err != nil {
		return api.ReleaseOutput{}, err
	}
	return api.ReleaseOutput{Release: toRelease(out)}, nil
}
