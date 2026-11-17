// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package proof

import "errors"

// sealedHere names how this platform seals the secret: it does not yet, so
// nothing is enrolled and every proof fails closed.
const sealedHere = ""

var errNoSeal = errors.New("no sealed store on this platform yet: an authenticator code cannot be enrolled here")

func seal([]byte) ([]byte, error)   { return nil, errNoSeal }
func unseal([]byte) ([]byte, error) { return nil, errNoSeal }
