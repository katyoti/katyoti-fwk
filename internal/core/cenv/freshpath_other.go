// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package cenv

// registryPath: no registry outside Windows; FreshPath keeps the PATH.
func registryPath() []string { return nil }
