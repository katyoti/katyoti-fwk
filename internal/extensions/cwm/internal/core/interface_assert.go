// SPDX-License-Identifier: Apache-2.0

package core

import "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

// Compile-time assertion that *Service satisfies the full
// api.CWMService interface (all 51 methods). When the api adds /
// removes / changes a method signature, this assertion fires
// before any test runs.
var _ api.CWMService = (*Service)(nil)
