// SPDX-License-Identifier: Apache-2.0

package embedded

import (
	"errors"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

// provisionWait bounds each JetStream call while provisioning.
const provisionWait = 30 * time.Second

// provisionJetStream creates the canonical streams + KV buckets if missing,
// for a JetStream store of maxStore bytes (the server's effective limit, see
// storeLimit). A stream kept from an earlier start takes the byte bound this
// store gives when it can.
// Idempotent — safe to call on every startup.
func provisionJetStream(nc *natsgo.Conn, maxStore int64) error {
	// Creating a file-backed stream can take more than the client's default
	// 5 s per call on a loaded machine (seen under the full parallel test
	// suite, with an on-access scanner): provisioning runs once, at start —
	// give each call its time rather than fail the hub's start.
	js, err := nc.JetStream(natsgo.MaxWait(provisionWait))
	if err != nil {
		return fmt.Errorf("JetStream context: %w", err)
	}

	for _, cfg := range streamConfigs(maxStore) {
		// Look the stream up first: a create request for an existing stream
		// whose config differs is checked against the store as a new one.
		info, err := js.StreamInfo(cfg.Name)
		if errors.Is(err, natsgo.ErrStreamNotFound) {
			if _, err := js.AddStream(cfg); err != nil {
				return fmt.Errorf("AddStream %s: %w", cfg.Name, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("StreamInfo %s: %w", cfg.Name, err)
		}
		// Stream already exists — verify subject filter matches.
		if !sameStringSlice(info.Config.Subjects, cfg.Subjects) {
			return fmt.Errorf("stream %s exists with different subject filter %v (want %v)",
				cfg.Name, info.Config.Subjects, cfg.Subjects)
		}
		// Bring its byte bound to what this store gives. Best effort: a store
		// that shrank below what the stream already reserves refuses the
		// update, and the stream, recovered as it was, keeps working.
		if cfg.MaxBytes > 0 && info.Config.MaxBytes != cfg.MaxBytes {
			resized := info.Config
			resized.MaxBytes = cfg.MaxBytes
			_, _ = js.UpdateStream(&resized)
		}
	}

	for _, cfg := range kvConfigs() {
		// nats.go v1.x KV does not expose a clean "already exists" sentinel
		// error; check existence first via KeyValue() and create only if
		// the bucket is not present.
		if _, err := js.KeyValue(cfg.Bucket); err == nil {
			// already exists
			continue
		} else if !errors.Is(err, natsgo.ErrBucketNotFound) {
			// surface unexpected errors (auth, server down, etc.)
			return fmt.Errorf("KeyValue %s lookup: %w", cfg.Bucket, err)
		}
		if _, err := js.CreateKeyValue(cfg); err != nil {
			return fmt.Errorf("CreateKeyValue %s: %w", cfg.Bucket, err)
		}
	}

	return nil
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
