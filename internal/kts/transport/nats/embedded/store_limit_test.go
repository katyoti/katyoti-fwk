// SPDX-License-Identifier: Apache-2.0

package embedded

import (
	"path/filepath"
	"testing"

	natsgo "github.com/nats-io/nats.go"
)

// startWithStore starts the embedded server on dir with a JetStream store of
// maxStore bytes: the condition of a small disk (a CI runner's /tmp, a user's
// nearly full drive), where nats-server sizes the store from the free space -
// set explicitly here, so the test sees the same store on every OS and disk.
func startWithStore(t *testing.T, dir string, maxStore int64) *EmbeddedHandle {
	t.Helper()
	opts := EmbeddedOptions(dir, true /*quiet*/)
	opts.JetStreamMaxStore = maxStore
	h, err := StartEmbedded(opts)
	if err != nil {
		t.Fatalf("StartEmbedded with a %d-byte store: %v", maxStore, err)
	}
	return h
}

// assertProvisioned checks every canonical stream and KV bucket exists and
// returns the session journal's byte bound.
func assertProvisioned(t *testing.T, h *EmbeddedHandle) int64 {
	t.Helper()
	js, err := h.Client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	var journal int64
	for _, name := range []string{StreamSessionJournal, StreamRegistration, StreamRoomHistory} {
		info, err := js.StreamInfo(name)
		if err != nil {
			t.Fatalf("StreamInfo %s: %v", name, err)
		}
		if name == StreamSessionJournal {
			journal = info.Config.MaxBytes
		}
	}
	for _, b := range []string{KVSession, KVRoom, KVPresence, KVSticky, KVDescriptor} {
		if _, err := js.KeyValue(b); err != nil {
			t.Fatalf("KeyValue %s: %v", b, err)
		}
	}
	return journal
}

// TestEmbedded_SmallStore_ProvisionsEverything — a JetStream store smaller
// than the journal's 10 GiB bound still starts the hub: every stream and
// bucket is provisioned and the journal is bounded by half the store
// (: a fixed 10 GiB reservation was refused on Linux CI's small
// disk with "insufficient storage resources available").
func TestEmbedded_SmallStore_ProvisionsEverything(t *testing.T) {
	const store = 256 << 20
	h := startWithStore(t, filepath.Join(t.TempDir(), "nats-jetstream"), store)
	t.Cleanup(h.Stop)
	if got := assertProvisioned(t, h); got != store/2 {
		t.Errorf("journal MaxBytes = %d, want %d (half the store)", got, store/2)
	}
}

// TestEmbedded_StoreShrinks_ResizesJournal — a journal kept from a start on a
// larger store takes the bound of a smaller store on the next start; a store
// shrunk below what the journal already reserves still starts the hub, the
// journal kept as it was.
func TestEmbedded_StoreShrinks_ResizesJournal(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nats-jetstream")
	first := startWithStore(t, dir, 1<<30)
	if got := assertProvisioned(t, first); got != 512<<20 {
		t.Fatalf("first start: journal MaxBytes = %d, want %d", got, 512<<20)
	}
	first.Stop()

	second := startWithStore(t, dir, 768<<20)
	if got := assertProvisioned(t, second); got != 384<<20 {
		t.Errorf("second start: journal MaxBytes = %d, want %d (half the smaller store)", got, 384<<20)
	}
	second.Stop()

	third := startWithStore(t, dir, 256<<20)
	t.Cleanup(third.Stop)
	if got := assertProvisioned(t, third); got != 384<<20 {
		t.Errorf("third start: journal MaxBytes = %d, want %d (kept: the store refuses the update)", got, 384<<20)
	}
}

func TestJournalBytes(t *testing.T) {
	for _, c := range []struct {
		store, want int64
	}{
		{0, journalMaxBytes},        // unknown store: the 10 GiB bound
		{-1, journalMaxBytes},       // unknown store
		{1 << 40, journalMaxBytes},  // a large disk: capped at 10 GiB
		{20 << 30, journalMaxBytes}, // exactly twice the bound
		{8 << 30, 4 << 30},          // a small disk: half the store
		{256 << 20, 128 << 20},      // a CI runner's store
	} {
		if got := journalBytes(c.store); got != c.want {
			t.Errorf("journalBytes(%d) = %d, want %d", c.store, got, c.want)
		}
	}
	if cfg := streamConfigs(256 << 20)[0]; cfg.Name != StreamSessionJournal || cfg.MaxBytes != 128<<20 || cfg.Storage != natsgo.FileStorage {
		t.Errorf("journal config = %s MaxBytes %d", cfg.Name, cfg.MaxBytes)
	}
}

// TestSizeStore — with no explicit store, the store is sized from the disk of
// its directory on every OS (nats-server alone gives 1 TiB on Windows), with
// a memory limit beside it so nats-server keeps the size; an explicit store
// is kept.
func TestSizeStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not-yet", "nats-jetstream")
	room := diskRoom(dir)
	if room <= 0 {
		t.Fatalf("diskRoom(%s) = %d, want the free space of its disk", dir, room)
	}
	opts := EmbeddedOptions(dir, true)
	sizeStore(opts)
	if want := room / 4 * 3; opts.JetStreamMaxStore <= 0 || opts.JetStreamMaxStore > want {
		t.Errorf("JetStreamMaxStore = %d, want about %d", opts.JetStreamMaxStore, want)
	}
	if opts.JetStreamMaxMemory <= 0 {
		t.Errorf("JetStreamMaxMemory = %d, want a limit beside the store's", opts.JetStreamMaxMemory)
	}

	h, err := StartEmbedded(opts)
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	t.Cleanup(h.Stop)
	if got := storeLimit(h.Server); got != opts.JetStreamMaxStore {
		t.Errorf("server store limit = %d, want the sized %d", got, opts.JetStreamMaxStore)
	}

	explicit := EmbeddedOptions(dir, true)
	explicit.JetStreamMaxStore = 256 << 20
	sizeStore(explicit)
	if explicit.JetStreamMaxStore != 256<<20 {
		t.Errorf("explicit store changed to %d", explicit.JetStreamMaxStore)
	}
}
