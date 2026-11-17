// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"encoding/json"
	"errors"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
)

// Standard JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// KTSP-specific error codes.
const (
	CodeSlotNotFound     = -40001
	CodeToolNotFound     = -40002
	CodeTaskNotFound     = -40003
	CodeResourceNotFound = -40004
	CodeContainerBusy    = -40005
)

// Standard JSON-RPC 2.0 errors.

func ErrParseError(detail string) *RPCError {
	return &RPCError{Code: CodeParseError, Message: "Parse error", Data: marshalDetail(detail)}
}

func ErrInvalidRequest(detail string) *RPCError {
	return &RPCError{Code: CodeInvalidRequest, Message: "Invalid request", Data: marshalDetail(detail)}
}

func ErrMethodNotFound(method string) *RPCError {
	return &RPCError{Code: CodeMethodNotFound, Message: "Method not found: " + method}
}

func ErrInvalidParams(detail string) *RPCError {
	return &RPCError{Code: CodeInvalidParams, Message: "Invalid params", Data: marshalDetail(detail)}
}

func ErrInternalError(detail string) *RPCError {
	// Embed the detail into Message so callers that only read Error()
	// see what went wrong without having to inspect Data. The Data
	// field is preserved for structured consumers.
	msg := "Internal error"
	if detail != "" {
		msg = "Internal error: " + detail
	}
	return &RPCError{Code: CodeInternalError, Message: msg, Data: marshalDetail(detail)}
}

// KTSP-specific errors.

func ErrSlotNotFound(slot string) *RPCError {
	return &RPCError{Code: CodeSlotNotFound, Message: "Slot not found: " + slot}
}

func ErrToolNotFound(tool string) *RPCError {
	return &RPCError{Code: CodeToolNotFound, Message: "Tool not found: " + tool}
}

func ErrTaskNotFound(taskID string) *RPCError {
	return &RPCError{Code: CodeTaskNotFound, Message: "Task not found: " + taskID}
}

func ErrResourceNotFound(uri string) *RPCError {
	return &RPCError{Code: CodeResourceNotFound, Message: "Resource not found: " + uri}
}

func ErrContainerBusy() *RPCError {
	return &RPCError{Code: CodeContainerBusy, Message: "Container busy"}
}

// IsCode checks whether an RPCError has a specific error code.
func IsCode(err *RPCError, code int) bool {
	return err != nil && err.Code == code
}

func marshalDetail(detail string) json.RawMessage {
	if detail == "" {
		return nil
	}
	b, _ := json.Marshal(detail)
	return b
}

// errorData is RPCError.Data when the error carries a person-facing message
// key: the key and params under `msg`, the detail (the text Data held before)
// under `detail`.
type errorData struct {
	Msg    *usermsg.Msg    `json:"msg,omitempty"`
	Detail json.RawMessage `json:"detail,omitempty"`
}

// WithMsg attaches a person-facing message key to the error (Data.msg), so the
// GUI can say it in the person's language; Message stays the English text.
// A detail Data held moves under Data.detail.
func (e *RPCError) WithMsg(m usermsg.Msg) *RPCError {
	if e == nil || m.Key == "" {
		return e
	}
	d := errorData{Msg: &m}
	if len(e.Data) > 0 {
		var prior errorData
		if json.Unmarshal(e.Data, &prior) == nil && prior.Msg != nil {
			d.Detail = prior.Detail
		} else {
			d.Detail = e.Data
		}
	}
	e.Data, _ = json.Marshal(d)
	return e
}

// UserMsg is the message key Data.msg carries, if any: an RPCError is a
// usermsg.Carrier on both sides of the wire.
func (e *RPCError) UserMsg() (usermsg.Msg, bool) {
	if e == nil || len(e.Data) == 0 || e.Data[0] != '{' {
		return usermsg.Msg{}, false
	}
	var d errorData
	if json.Unmarshal(e.Data, &d) != nil || d.Msg == nil || d.Msg.Key == "" {
		return usermsg.Msg{}, false
	}
	return *d.Msg, true
}

// InputRefusal is an error that refuses the caller's input - a malformed
// request, an ambiguous id - rather than a fault of the service: FromError
// answers it as invalid params, which a caller tells apart from an internal
// error (and never retries). A pack's sentinel satisfies it with a method of
// its own (cwmapi.ErrInvalidInput), without importing ktsp.
type InputRefusal interface {
	error
	InputRefused() bool
}

// IsInputRefusal reports whether err, or an error it wraps, refuses the
// caller's input.
func IsInputRefusal(err error) bool {
	var r InputRefusal
	return errors.As(err, &r) && r.InputRefused()
}

// errInputRefused is the invalid-params answer to an input refusal. Unlike
// ErrInvalidParams, its Message keeps the error's text: a caller that reads
// only Error() (cwmapi.Restore, the GUI's text readers) still finds it.
func errInputRefused(detail string) *RPCError {
	return &RPCError{Code: CodeInvalidParams, Message: "Invalid params: " + detail, Data: marshalDetail(detail)}
}

// FromError is the RPCError a handler's error answers with: the RPCError
// itself when it is one, invalid params when it refuses the caller's input
// (InputRefusal), else an internal error with its text — and, either way, the
// message key the error carries (usermsg.Of) in Data.msg.
func FromError(err error) *RPCError {
	if err == nil {
		return nil
	}
	var rpcErr *RPCError
	if errors.As(err, &rpcErr) {
		if _, ok := rpcErr.UserMsg(); ok {
			return rpcErr
		}
		cp := *rpcErr
		rpcErr = &cp
	} else if IsInputRefusal(err) {
		rpcErr = errInputRefused(err.Error())
	} else {
		rpcErr = ErrInternalError(err.Error())
	}
	if m, ok := usermsg.Of(err); ok {
		rpcErr.WithMsg(m)
	}
	return rpcErr
}
