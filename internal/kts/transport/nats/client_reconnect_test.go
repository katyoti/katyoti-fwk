// SPDX-License-Identifier: Apache-2.0

package nats

import "testing"

// Every hook registered on a handle runs once per reconnect, in order, and
// registering after the first reconnect still counts for the next. The NATS
// callback is a one-liner onto fireReconnect; this is the part that can be
// wrong without a server.
func TestClientHandle_OnReconnect_FiresEveryHookInOrder(t *testing.T) {
	h := &ClientHandle{}
	var got []string
	h.OnReconnect(func() { got = append(got, "view") })
	h.OnReconnect(nil) // ignored, never a panic
	h.OnReconnect(func() { got = append(got, "audit") })

	h.fireReconnect()
	if len(got) != 2 || got[0] != "view" || got[1] != "audit" {
		t.Fatalf("first reconnect ran %v, want [view audit]", got)
	}

	h.OnReconnect(func() { got = append(got, "late") })
	h.fireReconnect()
	if len(got) != 5 || got[4] != "late" {
		t.Fatalf("second reconnect ran %v, want the late hook too", got)
	}

	var nilHandle *ClientHandle
	nilHandle.OnReconnect(func() {}) // a client with no link registers nothing and does not panic
}
