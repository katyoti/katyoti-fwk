// SPDX-License-Identifier: Apache-2.0

// Package concurrency holds CWM's multi-process coordination: the
// workspace lockfile at <root>/.lock with PID + lease TTL + heartbeat.
//
// Acquisition state machine (see ProcessLockfile.Acquire): missing →
// write ours; own PID → renew; holder dead → take over; holder alive
// but heartbeat wedged → take over; holder alive + fresh →
// ErrLockHeldByOtherProcess.
package concurrency
