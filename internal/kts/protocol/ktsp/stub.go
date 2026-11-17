// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// GenericStub is the low-level client-side proxy for a remote slot.
// It wraps a Sender and a base path, composing full method paths and
// classifying errors. Typed per-slot stubs delegate to GenericStub
// for the actual KTSP communication.
//
// Sender errors (transport failures) are wrapped in KTSPError.
// Remote errors (*RPCError from the server) pass through unchanged —
// they represent the remote method's response, not a communication failure.
type GenericStub struct {
	sender   Sender
	basePath string // e.g., "container/slot"
	info     SlotInfo
}

// NewGenericStub creates a stub targeting the given base path via the sender.
// The info parameter provides cached slot metadata (typically obtained at
// connection time or from the remote container's manifest).
func NewGenericStub(sender Sender, basePath string, info SlotInfo) *GenericStub {
	return &GenericStub{
		sender:   sender,
		basePath: basePath,
		info:     info,
	}
}

// Call sends a request to the named method and returns the raw result. opts
// override the per-call defaults (timeout / retries / backoff). Transport errors
// are wrapped in KTSPError; remote errors pass through.
func (s *GenericStub) Call(ctx context.Context, method string, params any, opts ...CallOption) (json.RawMessage, error) {
	path := s.basePath + "/" + method
	result, err := s.sender.Call(ctx, path, params, opts...)
	if err != nil {
		return nil, s.classifyError("call", path, err)
	}
	return result, nil
}

// Notify sends a fire-and-forget notification to the named method.
// Transport errors are wrapped in KTSPError; remote errors pass through.
func (s *GenericStub) Notify(ctx context.Context, method string, params any, opts ...CallOption) error {
	path := s.basePath + "/" + method
	err := s.sender.Notify(ctx, path, params, opts...)
	if err != nil {
		return s.classifyError("notify", path, err)
	}
	return nil
}

// Info returns the cached slot metadata.
func (s *GenericStub) Info() SlotInfo { return s.info }

// BasePath returns the stub's target path (e.g., "container/slot").
func (s *GenericStub) BasePath() string { return s.basePath }

// classifyError wraps transport/communication errors in KTSPError.
// RPCErrors (from the remote side) pass through unchanged — they are
// the remote method's response, not a communication failure.
func (s *GenericStub) classifyError(op, path string, err error) error {
	// RPCError = remote responded with an error → pass through
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		return err
	}

	// Context errors → timeout/cancellation
	if errors.Is(err, context.DeadlineExceeded) {
		return &KTSPError{Kind: ErrKindTimeout, Op: op, Remote: path, Cause: err}
	}
	if errors.Is(err, context.Canceled) {
		return &KTSPError{Kind: ErrKindTimeout, Op: op, Remote: path, Cause: err}
	}

	// Everything else → transport error
	return &KTSPError{Kind: ErrKindTransport, Op: op, Remote: path, Cause: err}
}

// CallTyped sends a request and unmarshals the result into the output type.
// This is the standard helper for typed per-slot stubs — each method is a
// one-liner: return CallTyped[OutType](stub, ctx, "method", input)
func CallTyped[Out any](stub *GenericStub, ctx context.Context, method string, params any) (Out, error) {
	var zero Out
	raw, err := stub.Call(ctx, method, params)
	if err != nil {
		return zero, err
	}
	var out Out
	if err := json.Unmarshal(raw, &out); err != nil {
		path := stub.basePath + "/" + method
		return zero, &KTSPError{
			Kind:   ErrKindDeserialize,
			Op:     "unmarshal",
			Remote: path,
			Cause:  fmt.Errorf("unmarshal %T: %w", zero, err),
		}
	}
	return out, nil
}
