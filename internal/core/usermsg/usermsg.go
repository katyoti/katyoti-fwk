// SPDX-License-Identifier: Apache-2.0

// Package usermsg carries a person-facing error's message as a KEY and its
// PARAMS beside its English text, so the GUI can say it in the person's
// language (its `errors` namespace: t('errors:'+key, params)) while the CLI,
// the logs and the agents keep reading the text.
//
// A service attaches the key where it refuses (Wrap, or a typed error with a
// UserMsg method); the KTSP layer carries it across a process boundary in
// RPCError.Data.msg; Of finds it on either side.
package usermsg

// Msg is a message by key: `<domain>.<name>` (cwm.objectNotFound), with the
// values the sentence names (an id, a type, a reason). Params are strings or
// numbers — what an i18next interpolation shows.
type Msg struct {
	Key    string         `json:"key"`
	Params map[string]any `json:"params,omitempty"`
}

// Carrier is an error that knows its message key: a typed refusal
// (cwmapi.SizeBudgetError) or an RPCError that crossed the wire with one.
type Carrier interface {
	UserMsg() (Msg, bool)
}

// Error is an error given a message key. Its text is the wrapped error's, so
// attaching a key changes nothing a reader of the text sees; errors.Is / As
// reach the wrapped error.
type Error struct {
	Msg Msg
	Err error
}

func (e *Error) Error() string { return e.Err.Error() }

func (e *Error) Unwrap() error { return e.Err }

// UserMsg makes Error a Carrier.
func (e *Error) UserMsg() (Msg, bool) { return e.Msg, true }

// Wrap gives err the message key and params. A nil err stays nil.
func Wrap(err error, key string, params map[string]any) error {
	if err == nil {
		return nil
	}
	return &Error{Msg: Msg{Key: key, Params: params}, Err: err}
}

// Of is the message key the error carries — the outermost Carrier in its
// chain that answers one.
func Of(err error) (Msg, bool) {
	for err != nil {
		if c, ok := err.(Carrier); ok {
			if m, ok := c.UserMsg(); ok && m.Key != "" {
				return m, true
			}
		}
		switch u := err.(type) {
		case interface{ Unwrap() error }:
			err = u.Unwrap()
		case interface{ Unwrap() []error }:
			for _, e := range u.Unwrap() {
				if m, ok := Of(e); ok {
					return m, true
				}
			}
			return Msg{}, false
		default:
			return Msg{}, false
		}
	}
	return Msg{}, false
}

// Is reports whether the error carries the given key.
func Is(err error, key string) bool {
	m, ok := Of(err)
	return ok && m.Key == key
}
