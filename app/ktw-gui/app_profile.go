// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	profileremote "github.com/katyoti/katyoti-fwk/internal/extensions/profile/remote"
)

// profile is the person using this GUI, read from the profile service in the
// admin universe; when that cannot be reached, the placeholder, so what the
// person does still goes through.
func (a *App) profile() profileapi.Profile {
	if sender, err := a.universeSender(admin.UniverseName); err == nil {
		if p, err := profileremote.NewRemote(sender).Get(a.ctx); err == nil && p.ID != "" {
			a.rememberPerson(p)
			return p
		}
	}
	return profileapi.Placeholder()
}
