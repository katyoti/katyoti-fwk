// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strconv"
	"strings"
)

// DebugPortEnv names the port the GUI's WebView2 opens for remote debugging:
// the clean-machine proof attaches to it over the Chrome DevTools Protocol and
// walks the screens by their element ids. Set only in the
// proof's environment; Chromium binds it to 127.0.0.1.
const DebugPortEnv = "KTW_GUI_DEBUG_PORT"

// debugPortArgs are the browser arguments the variable's value asks: a port
// number opens it, empty asks nothing, anything else is refused.
func debugPortArgs(v string) ([]string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	port, err := strconv.Atoi(v)
	if err != nil || port < 1024 || port > 65535 {
		return nil, fmt.Errorf("%s=%q is not a port number (1024-65535): no debugging port opened", DebugPortEnv, v)
	}
	return []string{fmt.Sprintf("--remote-debugging-port=%d", port)}, nil
}
