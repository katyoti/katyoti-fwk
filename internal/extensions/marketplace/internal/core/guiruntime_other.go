// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package core

// thisGUIRuntime: outside Windows the GUI needs no runtime of its own.
func thisGUIRuntime() *guiRuntime { return nil }
