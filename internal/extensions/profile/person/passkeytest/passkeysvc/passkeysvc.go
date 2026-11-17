// SPDX-License-Identifier: Apache-2.0

// Package passkeysvc gives tests the real PasskeyService, its store sealed by
// a test seal - what a test of the approval page verifies against. Apart from
// passkeytest (the software phone) so the passkey package's own tests can use
// the phone without a cycle.
package passkeysvc

import (
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/internal/passkey"
)

// Service is the real PasskeyService over a store in dir, sealed by a test
// seal (any platform).
func Service(dir string) api.PasskeyService {
	flip := func(b []byte) ([]byte, error) {
		out := make([]byte, len(b))
		for i := range b {
			out[i] = b[i] ^ 0x5a
		}
		return out, nil
	}
	return passkey.Service{S: passkey.New(dir, flip, flip)}
}
