// SPDX-License-Identifier: Apache-2.0

//go:build windows

package core

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// officeIdentities is where Office records the accounts signed in on this
// Windows user (one subkey per identity, `EmailAddress` value).
const officeIdentities = `Software\Microsoft\Office\16.0\Common\Identity\Identities`

// osEmails: the Office sign-in addresses, then the user principal name. The UPN
// is a login name and may not be a mailbox — it comes last, for the user to pick.
func osEmails() []api.EmailCandidate {
	var out []api.EmailCandidate
	if k, err := registry.OpenKey(registry.CURRENT_USER, officeIdentities, registry.ENUMERATE_SUB_KEYS); err == nil {
		names, _ := k.ReadSubKeyNames(-1)
		for _, n := range names {
			sk, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			if e, _, err := sk.GetStringValue("EmailAddress"); err == nil {
				out = append(out, api.EmailCandidate{Email: e, Source: "office"})
			}
			sk.Close()
		}
		k.Close()
	}
	if upn := userNameEx(windows.NameUserPrincipal); upn != "" {
		out = append(out, api.EmailCandidate{Email: upn, Source: "upn"})
	}
	return out
}

func userNameEx(format uint32) string {
	n := uint32(256)
	for i := 0; i < 2; i++ {
		buf := make([]uint16, n)
		err := windows.GetUserNameEx(format, &buf[0], &n)
		if err == nil {
			return windows.UTF16ToString(buf[:n])
		}
		if err != windows.ERROR_MORE_DATA {
			return ""
		}
	}
	return ""
}
