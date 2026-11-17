// SPDX-License-Identifier: Apache-2.0

package api

import "errors"

// ErrLocked is the refusal of an update on a locked installation: its
// versions move only when a release is installed on it (katyoti.yml `locked`).
var ErrLocked = errors.New("locked")
