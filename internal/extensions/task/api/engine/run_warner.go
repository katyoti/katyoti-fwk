// SPDX-License-Identifier: Apache-2.0

package engine

import "context"

type runWarnerKey struct{}

// WithRunWarner gives a run's context the way to warn on it: what went wrong
// around the run while it goes on (the task service shows it on the run and
// journals it). A machine reaches it from its step's context (WarnRun).
func WithRunWarner(ctx context.Context, warn func(reason string)) context.Context {
	return context.WithValue(ctx, runWarnerKey{}, warn)
}

// RunWarnerFrom is the run's warner; one that only drops the warning when the
// run has none (a machine run outside a task service).
func RunWarnerFrom(ctx context.Context) func(reason string) {
	if w, ok := ctx.Value(runWarnerKey{}).(func(string)); ok && w != nil {
		return w
	}
	return func(string) {}
}

// WarnRun warns on the context's run (see WithRunWarner).
func WarnRun(ctx context.Context, reason string) { RunWarnerFrom(ctx)(reason) }
