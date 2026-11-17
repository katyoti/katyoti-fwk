// SPDX-License-Identifier: Apache-2.0

package api

// Helper pointer constructors for config fields.

func IntPtr(v int) *int             { return &v }
func Float64Ptr(v float64) *float64 { return &v }
