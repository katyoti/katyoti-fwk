// SPDX-License-Identifier: Apache-2.0

package jsonrpc

// wire.go contains the JSON-RPC 2.0 wire format types.
// These are internal to the session — not exported.

import "encoding/json"

// wire is the JSON-RPC 2.0 frame on the wire.
type wire struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *wireError      `json:"error,omitempty"`
}

// wireError is a JSON-RPC 2.0 error object on the wire.
type wireError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// messageKind classifies an incoming JSON-RPC 2.0 message.
type messageKind int

const (
	responseKind messageKind = iota
	requestKind
	notificationKind
	unknownKind
)

// decode parses raw JSON into a wire frame and classifies it.
func decode(data json.RawMessage) (*wire, messageKind) {
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, unknownKind
	}
	hasID := w.ID != ""
	hasMethod := w.Method != ""
	hasResult := len(w.Result) > 0 || w.Error != nil

	switch {
	case hasID && hasResult:
		return &w, responseKind
	case hasID && hasMethod:
		return &w, requestKind
	case hasMethod:
		return &w, notificationKind
	default:
		return &w, unknownKind
	}
}

// encodeRequest builds a JSON-RPC 2.0 request frame.
func encodeRequest(id, method string, params json.RawMessage) (json.RawMessage, error) {
	w := wire{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	return json.Marshal(w)
}

// encodeNotification builds a JSON-RPC 2.0 notification frame.
func encodeNotification(method string, params json.RawMessage) (json.RawMessage, error) {
	w := wire{JSONRPC: "2.0", Method: method, Params: params}
	return json.Marshal(w)
}

// encodeResponse builds a JSON-RPC 2.0 response frame.
func encodeResponse(id string, result json.RawMessage, rpcErr *wireError) (json.RawMessage, error) {
	w := wire{JSONRPC: "2.0", ID: id, Result: result}
	if rpcErr != nil {
		w.Error = rpcErr
	}
	return json.Marshal(w)
}
