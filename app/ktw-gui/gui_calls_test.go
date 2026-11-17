// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"
)

// : a call the GUI gives up by its id has its context cancelled; an
// unknown id, or one already ended, changes nothing.
func TestAGivenUpCallHasItsContextCancelled(t *testing.T) {
	r := &callRegistry{running: map[string]context.CancelFunc{}}
	ctx, done := r.begin(context.Background(), "sec-1")
	other, doneOther := r.begin(context.Background(), "sec-2")
	defer doneOther()
	r.cancel("nope")
	if ctx.Err() != nil || other.Err() != nil {
		t.Fatal("an unknown id cancelled a call")
	}
	r.cancel("sec-1")
	if ctx.Err() == nil {
		t.Fatal("the call given up kept its context")
	}
	if other.Err() != nil {
		t.Fatal("giving up one call cancelled another")
	}
	done()
	r.cancel("sec-1")
	if len(r.running) != 1 {
		t.Fatalf("ended calls left in the registry: %v", r.running)
	}
}
