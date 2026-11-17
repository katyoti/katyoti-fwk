// SPDX-License-Identifier: Apache-2.0

package main

import "embed"

//go:embed embed/tasks embed/data-types embed/deck-themes embed/deck-assets embed/cwm embed/skills embed/guide
var embeddedFS embed.FS
