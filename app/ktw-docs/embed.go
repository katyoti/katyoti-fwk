// SPDX-License-Identifier: Apache-2.0

package main

import "embed"

//go:embed embed/config embed/docs embed/filters embed/skills embed/elk-layouts embed/references embed/drawio embed/data-types embed/contribution-types embed/tasks embed/content-templates embed/slide-groups
var embeddedFS embed.FS
