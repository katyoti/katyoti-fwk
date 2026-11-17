// SPDX-License-Identifier: Apache-2.0

package api

import "github.com/katyoti/katyoti-fwk/internal/core/meta"

// ProfileServiceDef declares the profile surface. Served in the admin universe
// by ktw-univers (`ktw-univers:profile/<method>`); auto-CLI is suppressed — the
// `katyoti profile` command owns the CLI.
var ProfileServiceDef = meta.ServiceDef{
	Name:    "Profile",
	BusName: "profile",
	Short:   "The profile of the person using the platform",
	Long:    `Who the person using KaTyoti says they are: a stable handle (the person actor id), first and last name, email, and the language they read (empty follows the operating system). A local profile is an unverified claim; until one is set, the user is the John Doe placeholder. Login binds to this profile later. The profile also holds the guide's completions: the recipes the person completed.`,
	Version: "1.3.0",
	Methods: map[string]meta.MethodDef{
		"Get":     meta.MethodLong("Read the profile", "The saved profile, or the John Doe placeholder (source: placeholder) when none is set."),
		"Update":  meta.MethodLong("Set the profile", "Validates the handle, the email and the language, saves the profile (source: local), and announces the change."),
		"Suggest": meta.MethodLong("Suggest a profile from the OS", "What the hub's OS user is known as (account name, display name, email candidates). Nothing is saved."),
		"GuideGet": {
			Short:    "Read the guide completions",
			Long:     "The recipes this person completed, one entry per recipe, with when and how (checked, seen, skipped).",
			ReadOnly: true,
		},
		"GuideComplete": meta.MethodLong("Record a completed recipe", "Records that a recipe was completed (checked, seen or skipped), replacing an earlier entry for the same recipe, and announces the change."),
		"GuideReset":    meta.MethodLong("Forget guide completions", "Forgets one recipe's completion, or every completion when no recipe is given, and announces the change."),
		"RelayGet": {
			Short:    "Read the relay switch",
			Long:     "Whether an agent's session may relay the person's own acts once the person confirmed each one; off until the person turns it on.",
			ReadOnly: true,
		},
		"RelaySet": meta.MethodLong("Turn relaying on or off", "Lets an agent's session relay the person's own acts - each one confirmed by the person - or stops it. The person's own act: refused from an agent's session."),
	},
}
