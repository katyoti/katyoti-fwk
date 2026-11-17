// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
	termadapter "github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/adapter"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/room"
)

// refusingTasks refuses every run as a task whose model's key is not set is
// refused: the agent factory's error, keyed, wrapped by the task service.
type refusingTasks struct{ taskapi.TaskService }

func (refusingTasks) Run(context.Context, taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	why := errors.New("anthropic: key ANTHROPIC_FOUNDRY_API_KEY is not set — import it")
	keyed := usermsg.Wrap(why, "agent.modelKeyMissing", map[string]any{"model": "foundry-haiku", "reason": why.Error()})
	return taskapi.TaskRunResponse{}, fmt.Errorf("create machine: agent factory: create model: %w", keyed)
}

// : the person's case - no key imported - is refused when the run
// starts, before any model call: its chat gets the reason with its key, for
// the GUI to say it in the person's language with the next step, its session
// ends "error" and the run's end says the error (the Done the GUI reads).
func TestARunRefusedForANotSetKeySaysItsKeyAndEndsFailed(t *testing.T) {
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	r := room.NewRoom(t.TempDir(), nil)
	t.Cleanup(func() { r.Close() })
	application.PutService[*room.Room](app, "default", r)
	application.PutService[taskapi.TaskService](app, "default", refusingTasks{})
	ctx := application.WithApplication(context.Background(), app)

	server, client := termadapter.NewChannelPair(64)
	t.Cleanup(func() { server.Close() })
	ch, err := client.Receive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = ServeTask(ctx, server, TaskOpts{TaskName: "new-study", SessionID: "s-refused", SkipHumanGate: true})
	if err == nil || !strings.Contains(err.Error(), "is not set") {
		t.Fatalf("the run ends with %v; want its refusal", err)
	}
	var said *terminal.SystemContent
	for said == nil {
		select {
		case raw := <-ch:
			var ev terminal.MsgNewEvent
			if json.Unmarshal(raw, &ev) == nil && ev.Type == "msg.new" {
				var sc terminal.SystemContent
				ev.Msg.Content.As(&sc)
				if sc.Level == "error" {
					said = &sc
				}
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no error line in the chat")
		}
	}
	if said.Msg == nil || said.Msg.Key != "agent.modelKeyMissing" || said.Msg.Params["model"] != "foundry-haiku" || !strings.Contains(said.Message, "is not set") {
		t.Fatalf("the chat says %+v; want the reason with agent.modelKeyMissing", said)
	}
	if sess, _ := r.GetSession("s-refused"); sess.Status != "error" {
		t.Fatalf("the session ends %q; want error", sess.Status)
	}
}
