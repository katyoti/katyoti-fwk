// SPDX-License-Identifier: Apache-2.0

package main

// errString renders an error for a Wails result struct, where the error travels
// INSIDE the struct as a string (the GUI binding convention) rather than as a
// second return value.
//
// It lived in app_marketplace.go until that file was deleted — the marketplace
// panel now calls the generated typed KTSP client over the single generic
// dispatcher, so its 18 forwarding bindings were removed. The helper is still
// used by the bindings that legitimately remain (process lifecycle, native
// dialogs, GUI-local state), so it moved here rather than being deleted with them.
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
