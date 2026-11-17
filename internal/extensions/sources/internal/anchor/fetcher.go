// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	stdctx "context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Fetcher loads the raw content of a source URI. Implementations
// register themselves under a URI scheme (file, http, confluence,
// gdrive, …). The substrate ships only the file:// fetcher;
// method packs add the rest at Extension boot time via
// `Service.RegisterFetcher`.
//
// Fetch is called from the Reanchor service's goroutine; it MUST
// honor ctx cancellation and SHOULD timeout for network sources.
type Fetcher interface {
	Scheme() string
	Fetch(ctx stdctx.Context, uri string) ([]byte, error)
}

// FileFetcher reads a `file://` URI, and only inside the project: the project
// root comes from the context (WithProjectRoot), and a path that leaves it — an
// absolute path elsewhere, a `..` escape, a symlink pointing out — is refused.
// A source is a document the project depends on; reading any file the process
// can reach would turn an object field into a way to exfiltrate one.
type FileFetcher struct{}

// Scheme returns "file".
func (FileFetcher) Scheme() string { return "file" }

// Fetch reads the bytes at the path encoded in uri.
//
// Accepted forms:
//
//   - `file://./docs/spec.md` and `file://docs/spec.md` — relative to the
//     project root;
//   - `file:///C:/proj/docs/spec.md` (Windows) or `file:///proj/docs/spec.md`
//     (POSIX) — absolute, and inside the project root.
func (FileFetcher) Fetch(ctx stdctx.Context, uri string) ([]byte, error) {
	path, err := confineFileURI(ProjectRootFrom(ctx), uri)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sources: read %s: %w", path, err)
	}
	return data, nil
}

type projectRootKey struct{}

// WithProjectRoot carries the project root the file fetcher is confined to.
func WithProjectRoot(ctx stdctx.Context, root string) stdctx.Context {
	return stdctx.WithValue(ctx, projectRootKey{}, root)
}

// ProjectRootFrom returns the project root the context carries, or "".
func ProjectRootFrom(ctx stdctx.Context) string {
	root, _ := ctx.Value(projectRootKey{}).(string)
	return root
}

// confineFileURI resolves a `file://` URI against the project root and refuses
// any path outside it, symlinks followed.
func confineFileURI(root, uri string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("%w: no project root known — the file fetcher reads only inside the project (uri=%q)", api.ErrInvalidInput, uri)
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("sources: project root %q: %w", root, err)
	}
	path, relative, err := resolveFileURI(uri)
	if err != nil {
		return "", err
	}
	if relative {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	if !within(root, path) {
		return "", fmt.Errorf("%w: %s is outside the project %s — the file fetcher reads only inside it", api.ErrInvalidInput, uri, root)
	}
	// A symlink inside the project may point out of it: compare the real paths.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		realRoot, rerr := filepath.EvalSymlinks(root)
		if rerr != nil {
			realRoot = root
		}
		if !within(realRoot, real) {
			return "", fmt.Errorf("%w: %s resolves to %s, outside the project %s — the file fetcher reads only inside it", api.ErrInvalidInput, uri, real, realRoot)
		}
	}
	return path, nil
}

// within reports whether path is root or below it.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveFileURI strips the `file://` prefix and returns a filesystem path,
// and whether it is relative to the project (`file://./x`, `file://x`). On
// Windows, `/C:/…` paths drop the leading slash that net/url leaves behind.
func resolveFileURI(uri string) (string, bool, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", false, fmt.Errorf("sources: parse %q: %w", uri, err)
	}
	if u.Scheme != "file" {
		return "", false, fmt.Errorf("sources: file fetcher got scheme %q (uri=%q)", u.Scheme, uri)
	}
	if u.Host != "" && u.Host != "localhost" {
		// `file://./docs/x.md` and `file://docs/x.md` put the first segment in Host.
		return filepath.FromSlash(u.Host + u.Path), true, nil
	}
	path := u.Path
	// Windows: net/url decodes `/C:/...` literally; strip leading slash.
	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	if !filepath.IsAbs(filepath.FromSlash(path)) {
		return filepath.FromSlash(path), true, nil
	}
	return filepath.FromSlash(path), false, nil
}

// schemeOf returns the URI scheme (lowercased). Used by Service
// to dispatch to the right Fetcher. Returns "" when the URI has
// no scheme; callers treat that as an error.
func schemeOf(uri string) string {
	if i := strings.Index(uri, "://"); i > 0 {
		return strings.ToLower(uri[:i])
	}
	return ""
}

// fetcherRegistry holds the per-scheme map. Service embeds one;
// kept as a separate type so the lock surface is small and the
// Service struct stays focused.
type fetcherRegistry struct {
	mu sync.RWMutex
	m  map[string]Fetcher
}

func newFetcherRegistry() *fetcherRegistry {
	return &fetcherRegistry{m: map[string]Fetcher{}}
}

func (r *fetcherRegistry) register(scheme string, f Fetcher) {
	if scheme == "" || f == nil {
		return
	}
	r.mu.Lock()
	r.m[strings.ToLower(scheme)] = f
	r.mu.Unlock()
}

func (r *fetcherRegistry) lookup(scheme string) (Fetcher, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.m[strings.ToLower(scheme)]
	return f, ok
}
