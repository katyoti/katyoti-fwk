// SPDX-License-Identifier: Apache-2.0

package main

import "embed"

//go:embed embed/config embed/skills embed/tasks all:embed/references embed/cwm
var embeddedFS embed.FS
