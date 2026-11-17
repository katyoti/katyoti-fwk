// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/core/redact"
)

// SessionRecorder owns correlated dual journals (LLM + User) and provides
// a shared monotonic sequence counter for total-ordering events across both.
//
// Each session gets its own subdirectory under sessionsDir:
//
//	{sessionsDir}/{ts}_{taskName}/
//	  llm.jsonl        — LLM-perspective journal
//	  user.jsonl       — user-perspective journal
//	  summary.json     — computed after session ends
type SessionRecorder struct {
	sessionsDir string
	sessionID   string // e.g. "2026-03-01T15-04-05_hello-world"
	sessionDir  string // full path to the session subdirectory

	seq *sessionCounter

	llmJournal  *Journal
	userJournal *UserJournal

	// unsubs collects EventBus unsubscribe closures registered via AddUnsub.
	// Drained on Close to prevent the recorder's handlers from outliving the run
	// (which would leak entries in MemoryEventBus.subs across long-lived processes).
	unsubs []func()

	// redactor scrubs secrets from every journal line before it reaches
	// disk (at-rest disclosure, threat-T10 redaction half). nil = no-op.
	// Set once during synchronous wire-up via SetRedactor, before the run
	// goroutine starts writing.
	redactor *redact.Redactor
}

// SetRedactor installs the secret scrubber applied to every journal line.
// Call during wire-up, before any Log call; nil disables redaction.
func (r *SessionRecorder) SetRedactor(red *redact.Redactor) {
	r.redactor = red
}

// NewSessionRecorder creates a recorder that manages both journals for a session.
// If sessionsDir is empty, recording is disabled (both journals are no-ops).
//
// When `sessionID` is non-empty the recorder uses it verbatim — callers
// pre-compute the id (`<timestamp>_<taskName>` form) at task-launch time
// so the returned `TaskRunResponse.SessionID` matches the on-disk
// session directory bit-exact. Empty → generate the same way (kept for
// callers that don't care, e.g. one-shot tests).
func NewSessionRecorder(sessionsDir, taskName, sessionID string) (*SessionRecorder, error) {
	r := &SessionRecorder{sessionsDir: sessionsDir}

	if sessionsDir == "" {
		r.seq = &sessionCounter{}
		// Disabled — create no-op journals (file is nil → Log calls are no-ops)
		r.llmJournal = &Journal{recorder: r}
		r.userJournal = &UserJournal{recorder: r}
		return r, nil
	}

	if sessionID == "" {
		sessionID = GenerateSessionID(taskName)
	}
	r.sessionID = sessionID
	r.sessionDir = filepath.Join(sessionsDir, r.sessionID)

	// Session journals capture raw LLM I/O — env vars, file contents,
	// secrets the agent has read into context. Tighten the POSIX mode so
	// they're not world-readable on a multi-user host (R10 / audit
	// medium-finding-3). Windows ignores POSIX bits; the bits we set become
	// ACL hints when other tools read them, but the host's ACL is the
	// effective gate on Windows.
	if err := os.MkdirAll(r.sessionDir, 0o700); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	// Journals open O_APPEND, NOT O_TRUNC: a delegating parent and its
	// in-process children SHARE one session (the child inherits the parent's
	// sessionID → same dir → same llm.jsonl). A child recorder created
	// mid-run must not truncate the file and wipe the parent's already-written
	// start events (session_start / task_started). O_APPEND also makes the two
	// recorders' concurrent writes land atomically at EOF, interleaving by
	// line instead of overwriting each other. Session ids are unique per
	// top-level run (UUIDv7), so there is no stale-session reuse to clear.
	//
	// LLM journal: {sessionDir}/llm.jsonl
	llmPath := filepath.Join(r.sessionDir, "llm.jsonl")
	llmFile, err := os.OpenFile(llmPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create LLM journal: %w", err)
	}
	r.llmJournal = &Journal{out: rotatingFile{file: llmFile, path: llmPath}, path: llmPath, recorder: r}

	// User journal: {sessionDir}/user.jsonl
	userPath := filepath.Join(r.sessionDir, "user.jsonl")
	userFile, err := os.OpenFile(userPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		llmFile.Close()
		return nil, fmt.Errorf("create user journal: %w", err)
	}
	r.userJournal = &UserJournal{out: rotatingFile{file: userFile, path: userPath}, path: userPath, recorder: r}
	r.seq = openSessionCounter(r.sessionDir, llmPath, userPath, rotatedPath(llmPath, 1), rotatedPath(userPath, 1))

	return r, nil
}

// NextSeq returns the next monotonic sequence number, shared across both journals.
func (r *SessionRecorder) NextSeq() int64 {
	return r.seq.n.Add(1)
}

// SessionID returns the session identifier (e.g. "2026-03-01T15-04-05_hello-world").
func (r *SessionRecorder) SessionID() string {
	return r.sessionID
}

// GenerateSessionID returns a fresh session id of the form
// `<uuidv7>_<bareTaskName>`. The UUIDv7 prefix is time-sortable
// (48-bit Unix-ms timestamp + random tail) so directory listings
// under `<DataDir>/sessions/` order by creation time without
// per-second collisions; the `_<bareTaskName>` suffix preserves
// grep-ability and keeps every existing
// `strings.Index(name, "_")` parser (app_sessions.go, ktw-utils
// journal/sessions.go) working without changes.
//
// `taskName` is normalised before being appended:
//   - URI prefix is stripped (`ktw-demo:hello-world` → `hello-world`,
//     `ktw-demo:/task/hello-world` → `hello-world`) — clients
//     sometimes pass the URI form before TaskService.resolveTask has
//     a chance to bare-ify it.
//   - Filesystem-illegal characters (Windows: `<>:"/\|?*`, also
//     control bytes) are replaced with `-` so the on-disk directory
//     can actually be created.
//
// Falls back to a deterministic `pid` form only when UUIDv7 generation
// fails (effectively impossible with google/uuid v1.6+).
func GenerateSessionID(taskName string) string {
	bare := bareTaskName(taskName)
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Sprintf("fallback-%d_%s", os.Getpid(), bare)
	}
	return fmt.Sprintf("%s_%s", id.String(), bare)
}

// bareTaskName extracts the task's bare name from any of the URI
// forms callers might supply, then sanitises it for use as a
// filesystem-path component.
func bareTaskName(name string) string {
	// Strip provider prefix (everything up to and including the last
	// `:`). Handles both `provider:name` (task URI) and the resource-
	// URI form `provider:/task/name` — for the latter, this leaves
	// `/task/name`, which we then collapse with the TrimPrefix below.
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.TrimPrefix(name, "/task/")

	// Sanitise: replace anything Windows can't accept in a path
	// segment + control bytes with `-`. The result still survives
	// the `strings.Index(name, "_")` parser on the consumer side
	// because `-` is path-safe and not the separator.
	return sanitisePathSegment(name)
}

func sanitisePathSegment(s string) string {
	if s == "" {
		return "unnamed"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r < 0x20:
			b.WriteByte('-')
		case r == '<' || r == '>' || r == ':' || r == '"' ||
			r == '/' || r == '\\' || r == '|' || r == '?' ||
			r == '*':
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SessionDir returns the full path to the session subdirectory (empty if disabled).
func (r *SessionRecorder) SessionDir() string {
	return r.sessionDir
}

// LLMJournal returns the LLM-perspective journal.
func (r *SessionRecorder) LLMJournal() *Journal {
	return r.llmJournal
}

// UserJournal returns the user-perspective journal.
func (r *SessionRecorder) UserJournal() *UserJournal {
	return r.userJournal
}

// LLMJournalPath returns the path to the LLM journal file (empty if disabled).
func (r *SessionRecorder) LLMJournalPath() string {
	return r.llmJournal.Path()
}

// UserJournalPath returns the path to the user journal file (empty if disabled).
func (r *SessionRecorder) UserJournalPath() string {
	return r.userJournal.Path()
}

// AddUnsub registers an EventBus unsubscribe closure that Close should call
// before tearing down journals. Pass the func returned by bus.Subscribe(...).
// AddUnsub is not goroutine-safe by itself; callers register all unsubs
// during the synchronous wire-up phase before the run goroutine starts.
func (r *SessionRecorder) AddUnsub(fn func()) {
	if fn == nil {
		return
	}
	r.unsubs = append(r.unsubs, fn)
}

// Close drains all registered unsubscribe closures, then closes both journals.
// Calling Close more than once is safe — unsubs are zeroed after the first drain.
func (r *SessionRecorder) Close() error {
	for _, unsub := range r.unsubs {
		unsub()
	}
	r.unsubs = nil

	var firstErr error
	if err := r.llmJournal.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := r.userJournal.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if r.sessionDir != "" {
		closeSessionCounter(r.sessionDir) // r.seq stays: a late Log after Close writes nothing
	}
	return firstErr
}

// A SESSION'S JOURNAL IS NUMBERED ONCE — its lines' seq go on from the last one
// written, whoever writes them: a run restored after a restart, a child run
// writing into its parent's session. One counter per session folder in this
// process, seeded from the journal when first opened.

type sessionCounter struct {
	n    atomic.Int64
	refs int
}

var sessionCounters = struct {
	sync.Mutex
	byDir map[string]*sessionCounter
}{byDir: map[string]*sessionCounter{}}

func openSessionCounter(dir string, journals ...string) *sessionCounter {
	sessionCounters.Lock()
	defer sessionCounters.Unlock()
	c := sessionCounters.byDir[dir]
	if c == nil {
		c = &sessionCounter{}
		c.n.Store(lastSeq(journals...))
		sessionCounters.byDir[dir] = c
	}
	c.refs++
	return c
}

func closeSessionCounter(dir string) {
	sessionCounters.Lock()
	defer sessionCounters.Unlock()
	if c := sessionCounters.byDir[dir]; c != nil {
		if c.refs--; c.refs <= 0 {
			delete(sessionCounters.byDir, dir)
		}
	}
}

var seqField = regexp.MustCompile(`"seq":(\d+)`)

// lastSeq is the highest seq the journals hold, read from their last 64 KB
// (every line carries one; the last lines hold the highest).
func lastSeq(journals ...string) int64 {
	var last int64
	for _, path := range journals {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		if st, err := f.Stat(); err == nil && st.Size() > 64<<10 {
			_, _ = f.Seek(st.Size()-64<<10, io.SeekStart)
		}
		b, _ := io.ReadAll(f)
		_ = f.Close()
		for _, m := range seqField.FindAllSubmatch(b, -1) {
			if n, err := strconv.ParseInt(string(m[1]), 10, 64); err == nil && n > last {
				last = n
			}
		}
	}
	return last
}
