// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
)

// AllowFunc returns a permission policy that always allows the tool to run.
// Use sparingly — this bypasses the fail-closed default.
func AllowFunc() PermissionFunc {
	return func(_ context.Context, _ json.RawMessage) PermissionResult {
		return PermissionResult{Behavior: PermissionAllow}
	}
}

// AskFunc returns a permission policy that always asks the user.
// If message is empty, the kernel falls back to a generic prompt.
func AskFunc(message string) PermissionFunc {
	return func(_ context.Context, _ json.RawMessage) PermissionResult {
		return PermissionResult{Behavior: PermissionAsk, Message: message}
	}
}

// DenyFunc returns a permission policy that always denies the tool.
// The message is surfaced in the error tool_result.
func DenyFunc(message string) PermissionFunc {
	return func(_ context.Context, _ json.RawMessage) PermissionResult {
		return PermissionResult{Behavior: PermissionDeny, Message: message}
	}
}

// PolicyFromOptions builds the default permission policy from a tool's
// declarative flags:
//
//	ReadOnly    → Allow
//	Destructive → Ask (escalated wording)
//	otherwise   → Ask
//
// This is the policy applied when a tool is built with no explicit
// WithPermissionCheck — i.e. the fail-closed default chain.
func PolicyFromOptions(opts ToolOptions) PermissionFunc {
	switch {
	case opts.ReadOnly:
		return AllowFunc()
	case opts.Destructive:
		return AskFunc("This tool performs an irreversible action. Confirm to proceed.")
	default:
		return AskFunc("")
	}
}
