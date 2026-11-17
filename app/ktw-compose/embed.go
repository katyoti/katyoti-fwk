// SPDX-License-Identifier: Apache-2.0

package main

import "embed"

//go:embed embed/skills embed/templates embed/references embed/tasks embed/cwm embed/permission-profiles
var embeddedFS embed.FS
