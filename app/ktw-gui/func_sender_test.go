// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"

	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// funcSender answers each call with a function of (path, input).
type funcSender func(path string, in any) (any, error)

func (f funcSender) Call(_ context.Context, path string, params any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	reply, err := f(path, params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(reply)
}

func (f funcSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }
