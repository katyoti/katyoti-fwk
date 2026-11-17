// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// A service's refusal reaches the GUI with its message key: the result's
// `error` keeps the English text, `errorMsg` the key and params the GUI
// translates (errors namespace).
func TestRefusalReachesTheGUIByKey(t *testing.T) {
	refusal := ktsp.ErrInternalError(`cwm: refused by hook: before_create x:gate/create: not in this module`).
		WithMsg(usermsg.Msg{Key: "cwm.refusedByHook", Params: map[string]any{"reason": "not in this module"}})
	sender := funcSender(func(path string, in any) (any, error) {
		if path == "ktw-agents:cwm/object-create" {
			return nil, refusal
		}
		return map[string]any{}, nil
	})
	_, err := objectCreateVia(context.Background(), sender, "billing", "todo", "billing.a", "A", "", "")
	res := WorkspaceObjectMutationResult{Error: err.Error(), ErrorMsg: errorMsgOf(err)}
	if res.ErrorMsg == nil || res.ErrorMsg.Key != "cwm.refusedByHook" || res.ErrorMsg.Params["reason"] != "not in this module" {
		t.Fatalf("errorMsg = %+v", res.ErrorMsg)
	}
	raw, _ := json.Marshal(res)
	var wire map[string]any
	_ = json.Unmarshal(raw, &wire)
	if wire["errorMsg"].(map[string]any)["key"] != "cwm.refusedByHook" || wire["error"] == "" {
		t.Errorf("wire = %s", raw)
	}

	// A transition refused: Message and MessageMsg together.
	moveSender := funcSender(func(path string, in any) (any, error) {
		if path == "ktw-agents:cwm/object-transition" {
			return nil, ktsp.ErrInternalError("refused").WithMsg(usermsg.Msg{Key: "cwm.transitionNone"})
		}
		return map[string]any{"lifecycles": []any{}}, nil
	})
	out := objectTransitionVia(context.Background(), moveSender, "billing.a", "", "done", "", "", false, "")
	if out.MessageMsg == nil || out.MessageMsg.Key != "cwm.transitionNone" || out.Message == "" {
		t.Errorf("lifecycles = %+v", out)
	}
	if errorMsgOf(ktsp.ErrInternalError("plain")) != nil {
		t.Error("an unkeyed error has a key")
	}
}
