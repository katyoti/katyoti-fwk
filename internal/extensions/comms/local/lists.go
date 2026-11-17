// SPDX-License-Identifier: Apache-2.0

package commslocal

import (
	"context"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	commsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comms/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// ParentLists answers the scheduler who may take a child's question
// : a parent run's holding list has one member, the parent's
// run - an agent in a person's seat. Any other group is not one of them.
var ParentLists = schedapi.GroupMembers{Name: "comms-parent-lists", Members: parentListMembers}

func parentListMembers(_ context.Context, group string) ([]cwmapi.Actor, bool, error) {
	run, ok := commsapi.ParentOfList(group)
	if !ok || run == "" {
		return nil, false, nil
	}
	return []cwmapi.Actor{commsapi.ParentActor(run)}, true, nil
}

// ListsExtension contributes the parent runs' holding lists to the scheduler
// (group members); a binary without a scheduler takes
// none, and says so in its debug log.
func ListsExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("comms", ".parent-lists", func(app *application.Application) error {
			if err := application.Contribute(app, ParentLists); err != nil {
				slog.Debug("comms: no scheduler in this binary, the parent runs' lists are not contributed", "err", err)
			}
			return nil
		})
	}
}
