// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"os"
	"testing"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	taskcore "github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/core"
)

// TestMain points the application's data folder at a temporary one: a test
// that builds the task service through the application (NewTaskService) keeps
// its runs, results and journals there — never in the user's data folder,
// never read back by another test.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ktw-test-data-")
	if err != nil {
		panic(err)
	}
	os.Setenv("KTW_DATA", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// withLocalSpawner is the context an engine test builds a workflow with: its
// channel, and a spawner that runs children on it without the task service.
func withLocalSpawner(channel *engine.Channel) context.Context {
	return engine.WithSpawner(engine.WithChannel(context.Background(), channel), taskcore.NewLocalSpawner(channel))
}
