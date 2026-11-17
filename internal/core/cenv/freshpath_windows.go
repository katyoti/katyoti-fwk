// SPDX-License-Identifier: Apache-2.0

//go:build windows

package cenv

import "golang.org/x/sys/windows/registry"

const machineEnvKey = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`

// registryPath is the machine's PATH then the user's, expanded, as Windows
// gives a new process; an unreadable one is left out.
func registryPath() []string {
	var out []string
	for _, k := range []struct {
		root registry.Key
		path string
	}{{registry.LOCAL_MACHINE, machineEnvKey}, {registry.CURRENT_USER, "Environment"}} {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		v, _, err := key.GetStringValue("Path")
		key.Close()
		if err != nil {
			continue
		}
		if x, err := registry.ExpandString(v); err == nil {
			v = x
		}
		out = append(out, v)
	}
	return out
}
