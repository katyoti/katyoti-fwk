// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// The TaskGroups.yaml a new universe starts with reads as the task groups'
// config and declares the groups the binary's own runs use: a person's (the
// default) and the planner's — a group not declared does not exist, so a
// planner's run would otherwise fall into a person's group.
func TestTheShippedTaskGroupsDeclareTheGroupsInUse(t *testing.T) {
	b, err := embeddedFS.ReadFile("embed/config/TaskGroups.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg taskapi.TaskGroupsConfig
	if err := config.DecodeBytes(b, marshal.YAML, &cfg); err != nil {
		t.Fatalf("TaskGroups.yaml does not read: %v", err)
	}
	for _, g := range []string{taskapi.DefaultGroup, planapi.PlanGroup} {
		if _, ok := cfg.Groups[g]; !ok {
			t.Errorf("group %q not declared", g)
		}
	}
	if cfg.DefaultGroup != taskapi.DefaultGroup || cfg.MaxRunning <= 0 {
		t.Errorf("default group %q, universe max_running %d", cfg.DefaultGroup, cfg.MaxRunning)
	}
}
