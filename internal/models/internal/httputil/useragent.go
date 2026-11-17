// SPDX-License-Identifier: Apache-2.0

package httputil

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

var (
	userAgent     string
	userAgentOnce sync.Once
)

// UserAgent returns the User-Agent string for HTTP requests.
// Format: program/version ktw/version Go/version (GOOS GOARCH)
func UserAgent() string {
	userAgentOnce.Do(func() {
		var parts []string

		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Path != "" && info.Main.Path != "command-line-arguments" {
				name := info.Main.Path[strings.LastIndex(info.Main.Path, "/")+1:]
				parts = append(parts, name+"/"+info.Main.Version)
			}
		}

		parts = append(parts, fmt.Sprintf("Go/%s", runtime.Version()))
		parts = append(parts, fmt.Sprintf("(%s %s)", runtime.GOOS, runtime.GOARCH))

		userAgent = strings.Join(parts, " ")
	})
	return userAgent
}
