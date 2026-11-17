// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"time"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// defaultMetricInterval is how often TaskService publishes a
// RunQueueDepthEvent to the EventBus. 30s matches typical Prometheus
// scrape cadences; operators can override via SetMetricInterval.
const defaultMetricInterval = 30 * time.Second

// SetMetricInterval changes the periodic queue-depth emit cadence. Takes
// effect on the next tick. Pass 0 to suppress emission entirely (useful in
// tests that don't want background ticker noise).
func (s *Service) SetMetricInterval(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metricInterval = d
}

// runMetricLoop ticks every metricInterval and publishes a
// RunQueueDepthEvent reflecting the three run-state map sizes. Exits when
// either ctx is cancelled or metricStop is closed. A 0 interval skips
// emission entirely (still ticks once a minute to re-check the interval —
// keeps the goroutine responsive to runtime SetMetricInterval calls).
func (s *Service) runMetricLoop(ctx context.Context) {
	const recheckPeriod = time.Minute
	for {
		s.mu.RLock()
		interval := s.metricInterval
		s.mu.RUnlock()

		wait := interval
		if wait <= 0 {
			wait = recheckPeriod
		}

		select {
		case <-ctx.Done():
			return
		case <-s.metricStop:
			return
		case <-time.After(wait):
		}

		s.mu.RLock()
		curInterval := s.metricInterval
		s.mu.RUnlock()
		if curInterval <= 0 {
			continue // suppressed; just re-check on the next loop
		}

		s.emitQueueDepth()
	}
}

// emitQueueDepth samples the run-state maps and publishes the metric
// event. Each sample is taken under its own lock, so the counts may
// reflect a momentary skew of ±1 — acceptable for a gauge metric.
func (s *Service) emitQueueDepth() {
	runs := s.QueueDepth()
	var bg int
	if m, ok := s.bgMgr.(*bgManager); ok {
		bg = m.QueueDepth()
	}
	// Spawner is per-workflow, not service-scoped. We sample 0 here at
	// service level; callers that want spawner depth wire their own
	// observation per workflow run.
	event := &engine.RunQueueDepthEvent{
		Runs:              runs,
		BackgroundEntries: bg,
		SpawnerChildren:   0,
	}
	if s.channel != nil {
		_ = s.channel.Bus().Publish(engine.RunQueueDepthTopic(), event)
	}
}
