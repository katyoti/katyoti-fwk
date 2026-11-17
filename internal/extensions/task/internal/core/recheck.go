// SPDX-License-Identifier: Apache-2.0

package core

// EVENTS MISSED WHILE THE BUS WAS DOWN — core NATS keeps no message: an event
// published while this process was disconnected never arrives. When the bus
// reconnects, every run whose wait asks to be rechecked (RecheckOnResume: an
// agent awaiting an answer) is woken once to check for itself; it waits again
// if nothing came. Other waits (a workflow's wait for an event) are left as
// they are: a tick would be taken for their event.

// RecheckWaits wakes the waiting runs that recheck (see above).
func (s *Service) RecheckWaits() {
	if s.channel == nil {
		return
	}
	for _, id := range s.reg.ids() {
		var topic string
		s.reg.mutate(id, func(e *runEntry) {
			if e.wait != nil && e.wait.RecheckOnResume && len(e.wait.Conditions) > 0 && e.terminalAt.IsZero() {
				topic = e.wait.Conditions[0].Topic
			}
		})
		if topic != "" {
			s.channel.Bus().Publish(topic, map[string]any{"recheck": "the bus reconnected"})
		}
	}
}
