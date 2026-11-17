// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package server

// processRunning is only asked on Windows; elsewhere signal 0 answers.
func processRunning(int) bool { return true }
