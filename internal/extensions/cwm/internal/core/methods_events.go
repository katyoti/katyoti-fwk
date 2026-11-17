// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// EventsSubscribe delegates to the events sub-service. v1
// supports exact-subject + prefix-`.>` patterns against the
// in-process EventBus. NATS wildcards beyond that (`*` mid-
// pattern) land with the KTSP server-stream bridge in v2.
func (s *Service) EventsSubscribe(ctx stdctx.Context, in api.EventsSubscribeInput) (<-chan api.EventEnvelope, func(), error) {
	return s.Events.Subscribe(ctx, in)
}
