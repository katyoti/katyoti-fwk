// SPDX-License-Identifier: Apache-2.0

package embedded

import (
	"io/fs"
	"os"
	"path/filepath"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/server/sysmem"
	"github.com/shirou/gopsutil/v4/disk"
)

// sizeStore gives the JetStream store of opts an explicit size when none was
// set: three quarters of what the store directory's disk can hold - its free
// space plus what the store already uses, so the size does not shrink as the
// store fills. nats-server sizes it so itself on Linux but gives 1 TiB on
// Windows whatever the disk; sized here, the hub sees the real disk on every
// OS, and the session journal's byte bound follows it (journalBytes).
//
// nats-server keeps an explicit JetStreamMaxStore only with an explicit
// JetStreamMaxMemory beside it (without one it re-derives both), so the
// memory limit is set too when missing: three quarters of the system memory,
// nats-server's own default.
func sizeStore(opts *natsserver.Options) {
	if !opts.JetStream {
		return
	}
	if opts.JetStreamMaxStore <= 0 && opts.StoreDir != "" {
		if room := diskRoom(opts.StoreDir); room > 0 {
			opts.JetStreamMaxStore = room / 4 * 3
		}
	}
	if opts.JetStreamMaxStore > 0 && opts.JetStreamMaxMemory <= 0 {
		if m := sysmem.Memory(); m > 0 {
			opts.JetStreamMaxMemory = m / 4 * 3
		} else {
			opts.JetStreamMaxMemory = natsserver.JetStreamMaxMemDefault
		}
	}
}

// diskRoom is what the disk of storeDir can give the store: its free space
// plus the bytes already in storeDir. 0 when the disk cannot be read.
func diskRoom(storeDir string) int64 {
	dir := storeDir
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return 0
		}
		dir = parent
	}
	u, err := disk.Usage(dir)
	if err != nil || u == nil {
		return 0
	}
	return int64(u.Free) + dirBytes(storeDir)
}

// dirBytes sums the sizes of the files under dir (0 when it does not exist).
func dirBytes(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // a missing or unreadable entry counts for nothing
		}
		if d.Type().IsRegular() {
			if info, ierr := d.Info(); ierr == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// storeLimit is the JetStream store limit the server runs with: the
// JetStreamMaxStore of its options (see sizeStore), or the size nats-server
// derived itself when none was set. 0 when unknown.
func storeLimit(ns *natsserver.Server) int64 {
	if ns == nil {
		return 0
	}
	if c := ns.JetStreamConfig(); c != nil {
		return c.MaxStore
	}
	return 0
}
