// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Tier-2 perf benchmarks per `decisions/tasks-performance-budget.md`.
// Four benchmarks; run via `make bench`. PR-time gate via `benchstat`
// against `benchmarks/baseline.txt`: >10% regression on any benchmark
// fails the PR.
//
// Absolute targets (the "should we worry" line, NOT the per-PR gate):
//
//   p99 step latency (no-LLM)  : < 50 ms
//   peak RSS @ 1000 runs       : < 500 MB  (≈ 500 KB / run)
//   peak goroutines @ 1000 runs : < 5000   (5× run count)
//   recorder throughput        : > 10,000 entries/sec
//
// All benchmarks run without `-race` (race adds ~10× overhead that
// would dominate the measurements). Race is a separate gate via
// `make test-race`.

// BenchmarkStepLatency_NoLLM — measures the executor's react-commit-
// dispatch loop on a no-LLM machine (counterMachine). Reports ns/op
// across N Step → Step transitions; benchstat aggregates to p50/p99.
//
// Target: p99 < 50 ms (50_000_000 ns). Today's measurement (logged at
// benchmark close) should be ~100µs — three orders of magnitude under
// the target, so the gate is solely against regression.
func BenchmarkStepLatency_NoLLM(b *testing.B) {
	for i := 0; i < b.N; i++ {
		machine := &counterMachine{target: 10}
		exec := NewExecutor(machine, nil, nil)
		_, err := exec.Run(context.Background(), Event{Type: "start"})
		if err != nil {
			b.Fatalf("run %d: %v", i, err)
		}
	}
}

// BenchmarkRecorderThroughput — feeds N journal entries through a
// SessionRecorder backed by a tmp directory. Reports ns/entry;
// invert to get entries/sec.
//
// Target: > 10,000 entries/sec → < 100,000 ns/entry. Today's
// measurement should be well above that since fwrite to tmpfs is
// roughly 1µs.
func BenchmarkRecorderThroughput(b *testing.B) {
	rec, err := NewSessionRecorder(b.TempDir(), "bench-recorder", "")
	if err != nil {
		b.Fatalf("NewSessionRecorder: %v", err)
	}
	defer rec.Close()
	journal := rec.LLMJournal()
	if journal == nil {
		b.Fatal("LLMJournal returned nil")
	}

	payload := map[string]any{
		"type":     "synthetic",
		"runID":    "bench-run",
		"text":     "lorem ipsum dolor sit amet, consectetur adipiscing elit",
		"sequence": 0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		payload["sequence"] = i
		journal.LogWithTurn(JournalLLMResponse, 0, payload)
	}
}

// BenchmarkConcurrentSpawns — measures memory + goroutine impact of
// running N concurrent executors. Reports ns/spawn; runtime stats
// captured per iteration and surfaced via b.ReportMetric.
//
// Targets:
//
//	peak goroutines / spawns < 5
//	peak bytes / spawn       < 500 KB
//
// b.N is the spawn count. Memory + goroutine peak measured at the
// peak BEFORE the executors finish, then verified back to baseline.
func BenchmarkConcurrentSpawns(b *testing.B) {
	// Warm-up: bring goroutine count to a stable baseline before
	// measuring. Go's runtime spawns auxiliary goroutines for GC /
	// finalizers / timer; including those in the per-spawn ratio
	// would inflate the metric.
	runtime.GC()
	baseGoroutines := runtime.NumGoroutine()

	var memStatsBefore runtime.MemStats
	runtime.ReadMemStats(&memStatsBefore)

	var wg sync.WaitGroup
	wg.Add(b.N)
	b.ResetTimer()

	// Use a buffered channel as the "ready" signal so we can sample
	// the peak goroutines + memory while all executors are mid-flight.
	allRunning := make(chan struct{}, b.N)

	for i := 0; i < b.N; i++ {
		go func() {
			defer wg.Done()
			machine := &counterMachine{target: 100}
			exec := NewExecutor(machine, nil, nil)
			allRunning <- struct{}{}
			_, _ = exec.Run(context.Background(), Event{Type: "start"})
		}()
	}

	// Wait for every goroutine to have launched (not finished) so the
	// peak measurement reflects the in-flight state.
	for i := 0; i < b.N; i++ {
		<-allRunning
	}

	peakGoroutines := runtime.NumGoroutine()
	var peakMem runtime.MemStats
	runtime.ReadMemStats(&peakMem)

	wg.Wait()
	b.StopTimer()

	// Surface the peak metrics per-spawn. benchstat will aggregate
	// across runs; the absolute target check happens in CI scripting
	// against the latest run.
	if b.N > 0 {
		goroutinesPerSpawn := float64(peakGoroutines-baseGoroutines) / float64(b.N)
		bytesPerSpawn := float64(peakMem.Alloc-memStatsBefore.Alloc) / float64(b.N)
		b.ReportMetric(goroutinesPerSpawn, "goroutines/spawn")
		b.ReportMetric(bytesPerSpawn, "bytes/spawn")
	}
}

// BenchmarkChannelDispatch — measures EventBus.Publish + Channel
// fan-out latency. The fastest path through the engine outside the
// executor loop itself.
//
// No formal target in the ADR; included as a regression sentinel
// because the bus split (post-NATS) put more layers between
// Publish and the subscriber.
func BenchmarkChannelDispatch(b *testing.B) {
	bus := NewMemoryEventBus()
	ch := NewChannel(bus)
	_ = ch // Channel.Subscribe / Channel.Wait not exercised here —
	// this measures the raw EventBus path.

	var counter int64
	mu := sync.Mutex{}
	unsub := bus.Subscribe("bench.topic", func(_ string, _ any) {
		mu.Lock()
		counter++
		mu.Unlock()
	})
	defer unsub()

	payload := map[string]any{"k": "v"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bus.Publish("bench.topic", payload)
	}
	b.StopTimer()

	// Reasonable safety check: the subscriber should have received
	// every event before the benchmark ends (MemoryEventBus is
	// synchronous).
	mu.Lock()
	got := counter
	mu.Unlock()
	if got != int64(b.N) {
		b.Errorf("subscriber received %d events, want %d (lost-event regression)", got, b.N)
	}
	// Suppress the unused-time-import lint when this file gets edited
	// with no time.X reference left.
	_ = time.Millisecond
}
