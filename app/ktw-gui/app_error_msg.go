// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/katyoti/katyoti-fwk/internal/core/usermsg"

// errorMsgOf is the message key a service's refusal carries (usermsg, over
// KTSP in RPCError.Data.msg), for a binding result's `errorMsg`: the GUI says
// it in the person's language (`errors:<key>`), and shows `error`, the
// English text, when its locales lack the key. Nil when the error has none.
func errorMsgOf(err error) *usermsg.Msg {
	if m, ok := usermsg.Of(err); ok {
		return &m
	}
	return nil
}
