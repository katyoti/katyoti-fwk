// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package launcher

import "io"

// hideOwnConsole does nothing outside Windows: no shortcut opens a console there.
func hideOwnConsole(io.Writer) func(error) { return func(error) {} }
