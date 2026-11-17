// SPDX-License-Identifier: Apache-2.0

package main

import "embed"

// embeddedFS holds the README and the guide's shell recipes: the welcome
// tour and the recipes about the GUI shell. The hub is a member of every
// universe's view (its own manifest), so they are in the admin universe
// before any user universe exists AND in every universe an agent chats in
// (modules/guide, decision OD-011).
//
//go:embed README.md embed/guide
var embeddedFS embed.FS
