// SPDX-License-Identifier: Apache-2.0

// Stream + KV bucket configs for the hub's embedded NATS server.
// Constants only — never construct dynamically. Per
// `modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md`.
package embedded

import (
	"time"

	natsgo "github.com/nats-io/nats.go"
)

// Stream + KV bucket names per modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md.
// Constants only — never construct dynamically.
const (
	StreamSessionJournal = "KTW_SESSION_JOURNAL"
	StreamRegistration   = "KTW_REGISTRATION"
	StreamRoomHistory    = "KTW_ROOM_HISTORY"

	KVSession    = "KTW_SESSION"
	KVRoom       = "KTW_ROOM"
	KVPresence   = "KTW_PRESENCE"
	KVSticky     = "KTW_STICKY"
	KVDescriptor = "KTW_DESCRIPTOR"
)

// journalMaxBytes is the session journal's byte bound on a store that can
// hold it: 10 GiB.
const journalMaxBytes int64 = 10 << 30

// journalBytes is the session journal's byte bound for a JetStream store of
// maxStore bytes: 10 GiB, never more than half the store. JetStream reserves a
// file stream's MaxBytes against the store when the stream is created and
// refuses it when the store is smaller (a small disk: the store is sized from
// its free space), so a fixed 10 GiB would keep the hub from starting there;
// half the store leaves the other streams and the KV buckets their room. The
// journal stays bounded on every disk - MaxAge alone would let a busy hub fill
// the disk within its 30 days. maxStore <= 0 (unknown) keeps 10 GiB.
func journalBytes(maxStore int64) int64 {
	if maxStore > 0 && maxStore/2 < journalMaxBytes {
		return maxStore / 2
	}
	return journalMaxBytes
}

// streamConfigs returns the canonical stream provisioning list for a
// JetStream store of maxStore bytes (see journalBytes).
// Replicas==1 for embedded; cloud overrides at the StreamConfig level
// (currently the same configs work — the ADR notes R3 for cloud which
// is a deploy-time change, not a code-time one).
func streamConfigs(maxStore int64) []*natsgo.StreamConfig {
	return []*natsgo.StreamConfig{
		{
			Name:      StreamSessionJournal,
			Subjects:  []string{"ktw.*.universe.*.session.*.>"},
			Retention: natsgo.LimitsPolicy,
			MaxAge:    30 * 24 * time.Hour,
			MaxBytes:  journalBytes(maxStore),
			Storage:   natsgo.FileStorage,
			Replicas:  1,
		},
		{
			Name:              StreamRegistration,
			Subjects:          []string{"ktw.*.universe.*.registration.>"},
			Retention:         natsgo.LimitsPolicy,
			MaxMsgsPerSubject: 1, // last-message-per-subject semantics
			Storage:           natsgo.FileStorage,
			Replicas:          1,
		},
		{
			Name:      StreamRoomHistory,
			Subjects:  []string{"ktw.*.universe.*.room.*.session.>"},
			Retention: natsgo.LimitsPolicy,
			MaxAge:    365 * 24 * time.Hour,
			Storage:   natsgo.FileStorage,
			Replicas:  1,
		},
	}
}

// kvConfigs returns the canonical KV bucket provisioning list.
func kvConfigs() []*natsgo.KeyValueConfig {
	return []*natsgo.KeyValueConfig{
		{Bucket: KVSession, Replicas: 1, Storage: natsgo.FileStorage},
		{Bucket: KVRoom, Replicas: 1, Storage: natsgo.FileStorage},
		{Bucket: KVPresence, TTL: 30 * time.Second, Replicas: 1, Storage: natsgo.FileStorage},
		{Bucket: KVSticky, Replicas: 1, Storage: natsgo.FileStorage},
		{Bucket: KVDescriptor, Replicas: 1, Storage: natsgo.FileStorage},
	}
}
