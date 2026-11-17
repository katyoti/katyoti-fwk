// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"

	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// ModelCallLogObserver is the session sink for structured model-call
// events: each event lands as one JSON line in
// `<session>/llm-model-calls.jsonl` (the schema is llm.ModelCallEvent).
// The file opens lazily on the first event (creating the directory if
// needed); writes are line-atomic (one mutex-guarded marshal+append per
// event — SSE events arrive from the stream-consumer goroutine). The
// loop's terminal chokepoint closes it when the run ends.
type ModelCallLogObserver struct {
	mu   sync.Mutex
	path string
	f    *os.File
}

// NewModelCallLogObserver creates the sink for the given file path.
func NewModelCallLogObserver(path string) *ModelCallLogObserver {
	return &ModelCallLogObserver{path: path}
}

// ObserveModelCall implements llm.ModelCallObserver.
func (o *ModelCallLogObserver) ObserveModelCall(evt llm.ModelCallEvent) {
	line, err := json.Marshal(evt)
	if err != nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.f == nil {
		if err := os.MkdirAll(filepath.Dir(o.path), 0o755); err != nil {
			return
		}
		f, err := os.OpenFile(o.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
		o.f = f
	}
	_, _ = o.f.Write(append(line, '\n'))
}

// Close implements io.Closer; the loop calls it at the terminal
// chokepoint.
func (o *ModelCallLogObserver) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.f == nil {
		return nil
	}
	err := o.f.Close()
	o.f = nil
	return err
}

var (
	_ llm.ModelCallObserver = (*ModelCallLogObserver)(nil)
	_ io.Closer             = (*ModelCallLogObserver)(nil)
)
