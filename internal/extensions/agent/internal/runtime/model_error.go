// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"errors"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// The message keys of a failed model call, said in the person's language by
// the GUI (its `errors` namespace) with the next step to take.
const (
	MsgModelKeyMissing  = "agent.modelKeyMissing"
	MsgModelKeyRefused  = "agent.modelKeyRefused"
	MsgModelUnreachable = "agent.modelUnreachable"
	MsgModelUnknown     = "agent.modelUnknown"
	MsgModelCallFailed  = "agent.modelCallFailed"
)

// ModelCallError gives a failed model call - or a model that could not be
// created, its key not set - the message key of its cause: no key, a refused
// key, an unreachable endpoint, an unknown model, else a failed call, with the
// model and the error's text as params. Its text stays the error's own. The
// one classifier: the loop's calls and the agent factory's creation use it.
func ModelCallError(err error, model string) error {
	if err == nil {
		return nil
	}
	return usermsg.Wrap(err, modelCallKey(err), map[string]any{"model": model, "reason": err.Error()})
}

func modelCallKey(err error) string {
	text := strings.ToLower(err.Error())
	has := func(patterns ...string) bool {
		for _, p := range patterns {
			if strings.Contains(text, p) {
				return true
			}
		}
		return false
	}
	var le *llm.Error
	coded := errors.As(err, &le)
	switch {
	case has("is not set", "no api key", "missing api key", "api key is required", "api_key is required"):
		return MsgModelKeyMissing
	case coded && le.Code == llm.ErrCodeAuthentication,
		has("401", "403", "unauthorized", "forbidden", "invalid api key", "invalid x-api-key", "authentication", "permission denied"):
		return MsgModelKeyRefused
	case coded && le.Code == llm.ErrCodeResourceNotFound,
		has("model not found", "unknown model", "not_found_error", "does not exist", "404"):
		return MsgModelUnknown
	case coded && (le.Code == llm.ErrCodeProviderUnavailable || le.Code == llm.ErrCodeTimeout),
		has("no such host", "connection refused", "dial tcp", "i/o timeout", "tls:", "connection reset", "network is unreachable"):
		return MsgModelUnreachable
	}
	return MsgModelCallFailed
}
