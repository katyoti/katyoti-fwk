// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

type perManagerProbe struct{ owner int }

// The application scope's CONTAINER belongs to a ServiceManager, not to the
// process. Until 2026-09-22 NewServiceManager reassigned the package-level
// ApplicationScope, so every Application constructed in a process pointed the
// scope at its own container and stole it from every other one: a test that
// Put a service into its app and resolved it a microsecond later found
// "service not found" whenever a parallel test had constructed an app in
// between (TestGoPack_RunUnitTest_DispatchAndStatuses, flaky under
// `go test ./...`). Any process with two Applications — the census runs one
// per binary, the GUI one per universe — had the same hole.
func TestApplicationScope_ContainerIsPerManager(t *testing.T) {
	var wg sync.WaitGroup
	for owner := 0; owner < 8; owner++ {
		wg.Add(1)
		go func(owner int) {
			defer wg.Done()
			for round := 0; round < 200; round++ {
				sm := NewServiceManager(config.NewConfigService(loader.NewMapLoader(nil)))
				Put[*perManagerProbe](sm, "default", &perManagerProbe{owner: owner})
				got, err := Get[*perManagerProbe](sm, context.Background(), "default")
				if err != nil {
					t.Errorf("owner %d round %d: a service Put into this manager is not found in it: %v", owner, round, err)
					return
				}
				if got.owner != owner {
					t.Errorf("owner %d round %d: resolved another manager's service (owner %d)", owner, round, got.owner)
					return
				}
			}
		}(owner)
	}
	wg.Wait()
}

// A context stamped by one manager resolves that manager's container even when
// another manager was created later — the fallback is only for contexts that
// carry no application at all.
func TestApplicationScope_ContextWinsOverTheLatestManager(t *testing.T) {
	first := NewServiceManager(config.NewConfigService(loader.NewMapLoader(nil)))
	ctx := WithApplicationContainer(context.Background(), first.Container())
	_ = NewServiceManager(config.NewConfigService(loader.NewMapLoader(nil))) // later, unrelated

	if got := ApplicationScope.ContainerFromContext(ctx); got != first.Container() {
		t.Fatal("a context carrying the first manager's container resolved to another container")
	}
	if got := ApplicationScope.ContainerFromContext(context.Background()); got == first.Container() {
		t.Fatal("a bare context must fall back to the most recent manager, not the first")
	}
}
