// SPDX-License-Identifier: Apache-2.0

// KTSP L2 protocol message types: Request, Response, Notification.
//
// These are typed KTSP messages with three-level addressing
// (ContainerID/Slot/Method). They are NOT wire format types — the L1 wire
// encoding (JSON-RPC 2.0 envelope) is handled by transport/jsonrpc.
// The adapter assembles/disassembles the flat "method" string at the boundary.

package ktsp

import "encoding/json"

// Request is a KTSP request with three-level addressing.
type Request struct {
	ID          any             `json:"-"`
	ContainerID string          `json:"-"` // empty for local dispatch
	Slot        string          `json:"-"`
	Method      string          `json:"-"`
	Params      json.RawMessage `json:"params,omitempty"`
}

// Path returns the assembled path string (for logging, routing params, etc.).
func (r *Request) Path() string {
	return FormatPath(r.ContainerID, r.Slot, r.Method)
}

// Response is a KTSP response.
type Response struct {
	ID     any             `json:"-"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

// Notification is a KTSP notification (fire-and-forget, no response expected).
type Notification struct {
	ContainerID string          `json:"-"`
	Slot        string          `json:"-"`
	Method      string          `json:"-"`
	Params      json.RawMessage `json:"params,omitempty"`
}

// Path returns the assembled path string.
func (n *Notification) Path() string {
	return FormatPath(n.ContainerID, n.Slot, n.Method)
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e *RPCError) Error() string {
	return e.Message
}

// NewRequest creates a KTSP request from a path string.
// path is "container_id/slot/method" or "slot/method" (local).
// Params should be a struct, map, or json.RawMessage; nil for no params.
func NewRequest(id any, path string, params any) (*Request, error) {
	cid, slot, method := ParsePath(path)
	r := &Request{
		ID:          id,
		ContainerID: cid,
		Slot:        slot,
		Method:      method,
	}
	if params != nil {
		if raw, ok := params.(json.RawMessage); ok {
			r.Params = raw
		} else {
			p, err := json.Marshal(params)
			if err != nil {
				return nil, err
			}
			r.Params = p
		}
	}
	return r, nil
}

// NewResponse creates a successful KTSP response.
func NewResponse(id any, result any) (*Response, error) {
	r := &Response{ID: id}
	if result != nil {
		res, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		r.Result = res
	}
	return r, nil
}

// NewErrorResponse creates a KTSP error response.
func NewErrorResponse(id any, rpcErr *RPCError) *Response {
	return &Response{ID: id, Error: rpcErr}
}

// NewNotification creates a KTSP notification (fire-and-forget).
// path is "container_id/slot/method" or "slot/method" (local).
func NewNotification(path string, params any) (*Notification, error) {
	cid, slot, method := ParsePath(path)
	n := &Notification{
		ContainerID: cid,
		Slot:        slot,
		Method:      method,
	}
	if params != nil {
		if raw, ok := params.(json.RawMessage); ok {
			n.Params = raw
		} else {
			p, err := json.Marshal(params)
			if err != nil {
				return nil, err
			}
			n.Params = p
		}
	}
	return n, nil
}

// ParseParams unmarshals the Params field into the given target.
func (r *Request) ParseParams(target any) error {
	if r.Params == nil {
		return nil
	}
	return json.Unmarshal(r.Params, target)
}

// ParseResult unmarshals the Result field into the given target.
func (r *Response) ParseResult(target any) error {
	if r.Result == nil {
		return nil
	}
	return json.Unmarshal(r.Result, target)
}
