// SPDX-License-Identifier: Apache-2.0

package main

import "embed"

//go:embed embed/config embed/skills embed/data-types embed/tasks
var embeddedFS embed.FS
