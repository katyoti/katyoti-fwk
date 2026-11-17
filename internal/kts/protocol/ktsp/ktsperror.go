// SPDX-License-Identifier: Apache-2.0

package ktsp

import "fmt"

// KTSPError is a client-side infrastructure error produced by stubs
// when the KTSP communication itself fails (as opposed to the remote
// method returning a domain error). Use errors.As to distinguish
// infrastructure failures from domain errors.
//
// Domain errors from remote methods pass through unchanged — only
// transport, serialization, and protocol failures are wrapped.
type KTSPError struct {
	// Kind classifies the failure.
	Kind string

	// Op is the operation that failed.
	Op string

	// Remote is the target path (e.g., "container/slot/method").
	Remote string

	// Cause is the underlying error.
	Cause error
}

// Standard KTSPError kinds.
const (
	ErrKindTransport   = "transport"   // network/connection failure
	ErrKindSerialize   = "serialize"   // marshaling params failed
	ErrKindDeserialize = "deserialize" // unmarshaling result failed
	ErrKindTimeout     = "timeout"     // context deadline exceeded
	ErrKindProtocol    = "protocol"    // unexpected KTSP protocol error
)

func (e *KTSPError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("ktsp %s [%s] %s: %v", e.Kind, e.Op, e.Remote, e.Cause)
	}
	return fmt.Sprintf("ktsp %s [%s] %s", e.Kind, e.Op, e.Remote)
}

func (e *KTSPError) Unwrap() error { return e.Cause }
