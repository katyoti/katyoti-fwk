// SPDX-License-Identifier: Apache-2.0

package api

import "strings"

// Package kinds: the role a Go package plays, read from its path. Under an
// extension (internal/extensions/<ext>/<role>/…) it is the extension layout's
// role (modules/framework/extension-layout.md §2); elsewhere the area it sits
// in.
const (
	KindAPI      = "api"
	KindLocal    = "local"
	KindContrib  = "contrib"
	KindLint     = "lint"
	KindCommands = "commands"
	KindRemote   = "remote"
	KindPlugin   = "plugin"
	KindTesting  = "testing"
	KindInternal = "internal"
	KindCmd      = "cmd"
	KindMain     = "main"
	KindCore     = "core"
	KindTool     = "tool"
	KindOther    = "other"
)

// extensionRoles are the directory names the extension layout reserves.
var extensionRoles = map[string]bool{
	KindAPI: true, KindLocal: true, KindContrib: true, KindLint: true, KindCommands: true,
	KindRemote: true, KindPlugin: true, KindTesting: true, KindInternal: true,
}

// packageKindOverrides name the kind of a package its path does not tell,
// by repository-relative path (a package and those under it).
var packageKindOverrides = map[string]string{}

// PackageKind is the role of the Go package at path (repository-relative,
// forward slashes): an override when one is declared, else
//   - internal/extensions/<ext>/<role>/… → the role (api, local, contrib, lint,
//     commands, remote, plugin, testing, internal), a family member or a
//     deviation of the layout → internal;
//   - app/<bin>/cmd/… → cmd, app/<bin> → main, app/<bin>/<other>/… → internal;
//   - internal/test/… → testing, internal/<area>/… → core;
//   - tools/… → tool; anything else → other.
func PackageKind(path string) string {
	path = strings.Trim(strings.TrimPrefix(path, "./"), "/")
	best := ""
	for p := range packageKindOverrides {
		if (path == p || strings.HasPrefix(path, p+"/")) && len(p) > len(best) {
			best = p
		}
	}
	if best != "" {
		return packageKindOverrides[best]
	}
	seg := strings.Split(path, "/")
	switch {
	case len(seg) >= 3 && seg[0] == "internal" && seg[1] == "extensions":
		if len(seg) >= 4 && extensionRoles[seg[3]] {
			return seg[3]
		}
		return KindInternal
	case seg[0] == "app" && len(seg) == 2:
		return KindMain
	case seg[0] == "app" && len(seg) >= 3 && seg[2] == "cmd":
		return KindCmd
	case seg[0] == "app" && len(seg) >= 3:
		return KindInternal
	case seg[0] == "internal" && len(seg) >= 2 && seg[1] == "test":
		return KindTesting
	case seg[0] == "internal" && len(seg) >= 2:
		return KindCore
	case seg[0] == "tools":
		return KindTool
	}
	return KindOther
}
