// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package hubclient

func canDetectStalePIDsImpl() bool { return true }
