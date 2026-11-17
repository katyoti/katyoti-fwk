// SPDX-License-Identifier: Apache-2.0

package main

import "embed"

//go:embed embed/tasks embed/skills embed/config embed/references
var embeddedFS embed.FS
