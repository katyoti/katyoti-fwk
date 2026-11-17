// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"errors"
	"fmt"
	"testing"
)

func TestKTSPError_Error_WithCause(t *testing.T) {
	err := &KTSPError{
		Kind:   ErrKindTransport,
		Op:     "send",
		Remote: "container/slot/method",
		Cause:  fmt.Errorf("connection refused"),
	}
	got := err.Error()
	want := "ktsp transport [send] container/slot/method: connection refused"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestKTSPError_Error_WithoutCause(t *testing.T) {
	err := &KTSPError{
		Kind:   ErrKindTimeout,
		Op:     "send",
		Remote: "container/slot/method",
	}
	got := err.Error()
	want := "ktsp timeout [send] container/slot/method"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestKTSPError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("underlying error")
	err := &KTSPError{Kind: ErrKindTransport, Op: "send", Remote: "x", Cause: cause}
	if !errors.Is(err, cause) {
		t.Error("Unwrap should expose the cause")
	}
}

func TestKTSPError_ErrorsAs(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &KTSPError{
		Kind:   ErrKindSerialize,
		Op:     "send",
		Remote: "c/s/m",
		Cause:  fmt.Errorf("json error"),
	})

	var ktspErr *KTSPError
	if !errors.As(err, &ktspErr) {
		t.Fatal("errors.As should find KTSPError in chain")
	}
	if ktspErr.Kind != ErrKindSerialize {
		t.Errorf("got kind %q, want %q", ktspErr.Kind, ErrKindSerialize)
	}
}

func TestKTSPError_NotConfusedWithRPCError(t *testing.T) {
	var ktspErr *KTSPError
	rpcErr := &RPCError{Code: CodeInternalError, Message: "fail"}
	if errors.As(rpcErr, &ktspErr) {
		t.Error("RPCError should not match KTSPError via errors.As")
	}
}
