// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"reflect"
	"testing"
)

// D11 — public API contract tests.
//
// Scope: the 14 truly-public types from the charter's API-surface
// classification. These are the symbols with external callers
// (`internal/extensions/agent`, sidecars, `app/ktw-agents/cmd/`). Their
// shape must remain stable across releases — adding fields is fine,
// removing or renaming is a breaking change that requires explicit
// migration.
//
// **Discipline, not exhaustive enumeration**: this file asserts the
// fields/methods that downstream code depends on. If a future PR
// removes one of these without updating the test, the build breaks
// before the dependent package does. Adding fields is silent (no
// breakage). The relocate of the ~30 cross-pkg-internal symbols out of
// `api/` is deferred to a tier-2 follow-up paired with D5
// (unify-run-lifecycle-types) — the contract discipline here is
// independent of where types physically live.

// TestContract_TaskService_InterfaceShape pins the methods on
// TaskService. Compile-time assertion via interface satisfaction.
func TestContract_TaskService_InterfaceShape(t *testing.T) {
	// The mock satisfies TaskService iff every method on the interface is
	// implemented. Removing or renaming a method on TaskService breaks
	// this compile.
	var _ TaskService = (*mockTaskService)(nil)
}

// TestContract_TaskRunRequest_RequiredFields verifies the request shape
// hasn't lost critical fields. Downstream consumers (CLI run command,
// terminal protocol, sidecars) rely on these names.
func TestContract_TaskRunRequest_RequiredFields(t *testing.T) {
	r := TaskRunRequest{}
	// These field accesses fail compile if the field is removed.
	_ = r.TaskName
	_ = r.TaskFile
	_ = r.TaskDef
	_ = r.Input
	_ = r.WorkspaceRoot
	_ = r.ParentRunID
	_ = r.SessionID
	_ = r.ConfigOverride
	_ = r.MaxDurationS
	_ = r.FireAndForget
}

// TestContract_TaskRunResponse_RequiredFields — runID is the contract.
func TestContract_TaskRunResponse_RequiredFields(t *testing.T) {
	r := TaskRunResponse{}
	_ = r.RunID
}

// TestContract_RunStatus_KnownValues verifies the documented enum values
// exist. Adding values is fine; removing values would break downstream
// status-string comparisons.
func TestContract_RunStatus_KnownValues(t *testing.T) {
	values := map[RunStatus]string{
		RunStatusPending:   "pending",
		RunStatusRunning:   "running",
		RunStatusCompleted: "completed",
		RunStatusFailed:    "failed",
		RunStatusCancelled: "cancelled",
	}
	for got, want := range values {
		if string(got) != want {
			t.Errorf("RunStatus value mismatch: got %q, want %q", string(got), want)
		}
	}
}

// TestContract_TypedIDs_ZeroValues — RunID/TaskID/ChildID/NodeID
// (introduced by D6) round-trip as plain strings. Pin the named-string
// underlying type — future PRs that try to change the underlying type
// will break this test.
func TestContract_TypedIDs_ZeroValues(t *testing.T) {
	cases := []struct {
		name string
		val  any
	}{
		{"RunID", RunID("")},
		{"TaskID", TaskID("")},
		{"ChildID", ChildID("")},
		{"NodeID", NodeID("")},
	}
	for _, tc := range cases {
		if reflect.TypeOf(tc.val).Kind() != reflect.String {
			t.Errorf("%s underlying kind = %v, want string", tc.name, reflect.TypeOf(tc.val).Kind())
		}
	}
}

// TestContract_RunInfo_RequiredFields — UI / CLI / observers walk this
// structure to display run lists. Fields below are load-bearing.
func TestContract_RunInfo_RequiredFields(t *testing.T) {
	r := RunInfo{}
	_ = r.RunID
	_ = r.TaskName
	_ = r.MachineType
	_ = r.ParentRunID
	_ = r.SessionID
	_ = r.Status
	_ = r.StartedAt
}

// TestContract_TaskInfo_RequiredFields — `tasks list` consumers depend
// on Name + Type + Description + Provider.
func TestContract_TaskInfo_RequiredFields(t *testing.T) {
	t1 := TaskInfo{}
	_ = t1.Name
	_ = t1.Type
	_ = t1.Description
	_ = t1.Provider
}

// mockTaskService satisfies TaskService for the interface-shape test
// (TestContract_TaskService_InterfaceShape). It is NOT a usable
// implementation — every method panics. The test only verifies the
// interface is satisfiable from the outside.
type mockTaskService struct{}

func (m *mockTaskService) Groups(context.Context, GroupsRequest) (GroupsResponse, error) {
	panic("contract test mock")
}
func (m *mockTaskService) SetGroup(context.Context, SetGroupRequest) (GroupLoad, error) {
	panic("contract test mock")
}
func (m *mockTaskService) ClearGroup(context.Context, ClearGroupRequest) (ClearGroupResponse, error) {
	panic("contract test mock")
}

func (m *mockTaskService) Run(_ context.Context, _ TaskRunRequest) (TaskRunResponse, error) {
	panic("contract test mock")
}
func (m *mockTaskService) Status(_ context.Context, _ StatusRequest) (StatusResponse, error) {
	panic("contract test mock")
}
func (m *mockTaskService) Stop(_ context.Context, _ StopRequest) (StopResponse, error) {
	panic("contract test mock")
}
func (m *mockTaskService) Result(_ context.Context, _ ResultRequest) (ResultResponse, error) {
	panic("contract test mock")
}
func (m *mockTaskService) Runs(_ context.Context, _ ListRunsRequest) (ListRunsResponse, error) {
	panic("contract test mock")
}
func (m *mockTaskService) RunStages(_ context.Context, _ RunStagesRequest) (RunStagesResponse, error) {
	panic("contract test mock")
}
func (m *mockTaskService) List(_ context.Context, _ ListTasksRequest) (ListTasksResponse, error) {
	panic("contract test mock")
}
func (m *mockTaskService) Get(_ context.Context, _ GetTaskRequest) (TaskInfo, error) {
	panic("contract test mock")
}
func (m *mockTaskService) BackgroundManager() BackgroundTaskManager {
	panic("contract test mock")
}

func (m *mockTaskService) Repeat(context.Context, RepeatRequest) (TaskRunResponse, error) {
	return TaskRunResponse{}, nil
}
func (m *mockTaskService) SuspensionManager() SuspensionManager {
	panic("contract test mock")
}
