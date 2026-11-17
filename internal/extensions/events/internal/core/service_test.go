// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
)

// TestEventService_Subscribe_ParseErrorSurfaces confirms a malformed
// EventURI yields a clean error BEFORE the bus lookup is attempted.
// The bus is absent in this test (no MessageBus wired in the test
// context); errors must come from URI parsing, not from missing bus.
func TestEventService_Subscribe_ParseErrorSurfaces(t *testing.T) {
	svc, _ := New(context.Background(), &Config{})
	_, err := svc.Subscribe(context.Background(), "not-a-uri", "", nil)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "EventService.Subscribe") {
		t.Errorf("error %q missing facade prefix", err.Error())
	}
}

// TestEventService_Subscribe_NotEventURI_Rejects confirms a tool URI
// (`<provider>:<slot>/<method>`) is correctly rejected as the wrong
// URI flavour — only EventURIs (`:!`-discriminated) are accepted.
func TestEventService_Subscribe_NotEventURI_Rejects(t *testing.T) {
	svc, _ := New(context.Background(), &Config{})
	_, err := svc.Subscribe(context.Background(),
		"ktw-acme:tasks/dossier/refine-scope", "", nil)
	if err == nil {
		t.Fatal("expected error for tool URI passed as event URI")
	}
	if !strings.Contains(err.Error(), "event URI") {
		t.Errorf("error %q missing 'event URI' phrase", err.Error())
	}
}

// TestEventService_Publish_ParseErrorSurfaces mirrors the Subscribe
// parse-error test for Publish.
func TestEventService_Publish_ParseErrorSurfaces(t *testing.T) {
	svc, _ := New(context.Background(), &Config{})
	err := svc.Publish(context.Background(), "still-not-a-uri", "", "payload")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "EventService.Publish") {
		t.Errorf("error %q missing facade prefix", err.Error())
	}
}

// TestEventService_URIToSubject_MapsCorrectly pins the URI → subject
// resolution. cwm-v2-providers US-15: workspace scope subject must
// match the established NATS hierarchy.
func TestEventService_URIToSubject_MapsCorrectly(t *testing.T) {
	subject, err := uriToSubject(context.Background(),
		"cwm:!workspace/object/abc/created", "ws-1")
	if err != nil {
		t.Fatalf("uriToSubject: %v", err)
	}
	// The path segments + scope token must land in the right places.
	if !strings.Contains(subject, ".workspace.ws-1.object.abc.created") {
		t.Errorf("subject %q missing .workspace.ws-1.object.abc.created", subject)
	}
}

// TestEventService_URIToSubject_EmptyContextFallsBackToDefaults pins
// the in-process fallback: a context with no scope (the agent inbox
// tool's situation) must resolve tenant/universe via the canonical
// resolver so the NATS subject is valid AND matches the CWM
// publisher. Empty tokens (ktw..universe..) make NATS reject the
// subscribe, which is what stranded the inbox round-trip; a fallback
// diverging from the publisher's tokens strands it just as silently.
func TestEventService_URIToSubject_EmptyContextFallsBackToDefaults(t *testing.T) {
	subject, err := uriToSubject(context.Background(),
		"cwm:!workspace/object/abc/updated", "*")
	if err != nil {
		t.Fatalf("uriToSubject: %v", err)
	}
	// The exact tenant/universe depend on the test environment's
	// workspace manifest; what matters is that they match the
	// canonical resolver's output (publisher parity) and contain no
	// empty token (the shape NATS rejects).
	wantTenant, wantUniverse := cenv.ResolveTenantUniverse(application.Env().UniverseDir())
	want := "ktw." + wantTenant + ".universe." + wantUniverse + ".workspace.*.object.abc.updated"
	if subject != want {
		t.Errorf("subject = %q, want %q (canonical-resolver parity)", subject, want)
	}
	if strings.Contains(subject, "..") {
		t.Errorf("subject %q contains an empty token (consecutive dots)", subject)
	}
}

// Ensure the package's interface satisfaction stays intact at
// compile-time as the implementation evolves.
var _ eventsapi.EventService = (*Service)(nil)
