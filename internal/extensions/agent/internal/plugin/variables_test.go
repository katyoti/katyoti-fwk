// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"os"
	"testing"
)

func TestResolveVariables(t *testing.T) {
	vars := ResolveVariables("/workspace", "/config", "my-task", "sess-123")

	if vars["workspace"] != "/workspace" {
		t.Errorf("workspace: got %q", vars["workspace"])
	}
	if vars["config_dir"] != "/config" {
		t.Errorf("config_dir: got %q", vars["config_dir"])
	}
	if vars["task_name"] != "my-task" {
		t.Errorf("task_name: got %q", vars["task_name"])
	}
	if vars["session_id"] != "sess-123" {
		t.Errorf("session_id: got %q", vars["session_id"])
	}
	if _, ok := vars["user_home_dir"]; !ok {
		t.Error("user_home_dir should be set")
	}
	if _, ok := vars["cwd"]; !ok {
		t.Error("cwd should be set")
	}
}

func TestExpandVariables_BuiltIn(t *testing.T) {
	vars := map[string]string{
		"workspace": "/my/workspace",
		"task_name": "my-task",
	}

	result := ExpandVariables("path: ${workspace}/out/${task_name}", vars)
	expected := "path: /my/workspace/out/my-task"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestExpandVariables_Env(t *testing.T) {
	os.Setenv("TEST_KTW_VAR", "hello")
	defer os.Unsetenv("TEST_KTW_VAR")

	result := ExpandVariables("val=${env:TEST_KTW_VAR}", nil)
	if result != "val=hello" {
		t.Errorf("got %q, want %q", result, "val=hello")
	}
}

func TestExpandVariables_UnresolvedEnv(t *testing.T) {
	os.Unsetenv("TEST_KTW_MISSING")

	result := ExpandVariables("val=${env:TEST_KTW_MISSING}", nil)
	if result != "val=${env:TEST_KTW_MISSING}" {
		t.Errorf("unresolved env should be left as-is, got %q", result)
	}
}

func TestExpandVariables_UnresolvedVar(t *testing.T) {
	result := ExpandVariables("val=${unknown_var}", nil)
	if result != "val=${unknown_var}" {
		t.Errorf("unresolved var should be left as-is, got %q", result)
	}
}

func TestExpandVariables_NoVars(t *testing.T) {
	result := ExpandVariables("no variables here", nil)
	if result != "no variables here" {
		t.Errorf("got %q", result)
	}
}

func TestExpandVariables_MultipleVars(t *testing.T) {
	vars := map[string]string{
		"a": "AAA",
		"b": "BBB",
	}

	result := ExpandVariables("${a}-${b}", vars)
	if result != "AAA-BBB" {
		t.Errorf("got %q, want %q", result, "AAA-BBB")
	}
}
