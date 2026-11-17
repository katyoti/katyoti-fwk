// SPDX-License-Identifier: Apache-2.0

// Package proof is a person's authenticator code (level 2 of the relayed acts,
// ): an RFC 6238 time-based code from any authenticator app, its
// secret enrolled once and kept sealed on this machine (Windows DPAPI, the
// user's own key), apart from Profile.yaml, which clients read. A code proves
// one act, once: ±1 step of 30 s, a step at or below the last one used is
// refused, wrong codes slow the next attempts down (never a hard lock), every
// attempt is journaled, and with nothing enrolled every proof fails closed.
// The secret is never returned by anything but the enrolment itself.
package proof

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"time"
)

// period is a code's step, in seconds.
const period = 30

// digits is a code's length.
const digits = 6

// stepAt is the step a time falls in.
func stepAt(t time.Time) int64 { return t.Unix() / period }

// code is the RFC 6238 code (HMAC-SHA1) of secret at step, n digits long.
func code(secret []byte, step int64, n int) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < n; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", n, v%mod)
}

// match is the step within ±1 of now whose code is c, or false.
func match(secret []byte, c string, now time.Time) (int64, bool) {
	s := stepAt(now)
	for _, step := range []int64{s - 1, s, s + 1} {
		if hmac.Equal([]byte(code(secret, step, digits)), []byte(c)) {
			return step, true
		}
	}
	return 0, false
}
