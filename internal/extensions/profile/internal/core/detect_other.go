// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package core

import "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"

// osEmails: no OS-level email source outside Windows; git still applies.
func osEmails() []api.EmailCandidate { return nil }
