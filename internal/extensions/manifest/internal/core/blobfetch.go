// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

// blobfetch.go — fetching bulk content from a locator.
//
// One path serves every backend. A `file://` locator is what local development
// mints and a `https://` one is what an object store mints, and the consumer
// does not know or care which it got: the difference is a scheme, handled here,
// not a branch every caller repeats.
//
// Nothing here holds content. Every function streams, so a multi-gigabyte layer
// costs a buffer rather than its own size — which is the whole reason the bulk
// plane exists.

// FetchBlob opens a locator for reading, optionally from a byte offset.
//
// `offset` > 0 resumes: a file backend seeks, an HTTP backend sends a Range
// header. An interrupted transfer therefore continues from where it stopped
// instead of starting over — which matters in proportion to how large the
// content is, and bulk content is where it matters most.
func FetchBlob(ctx context.Context, locator string, offset int64) (io.ReadCloser, error) {
	u, err := url.Parse(locator)
	if err != nil {
		return nil, fmt.Errorf("blob fetch: parse locator %q: %w", locator, err)
	}
	switch u.Scheme {
	case "file", "":
		return fetchFile(u, offset)
	case "http", "https":
		return fetchHTTP(ctx, locator, offset)
	default:
		return nil, fmt.Errorf("blob fetch: unsupported locator scheme %q", u.Scheme)
	}
}

func fetchFile(u *url.URL, offset int64) (io.ReadCloser, error) {
	f, err := os.Open(filePathFromURL(u))
	if err != nil {
		return nil, fmt.Errorf("blob fetch: open %s: %w", u, err)
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("blob fetch: seek %s: %w", u, err)
		}
	}
	return f, nil
}

// filePathFromURL converts a file:// URL back to an OS path. Windows locators
// carry a leading slash before the drive letter, which has to come off.
func filePathFromURL(u *url.URL) string {
	p := u.Path
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func fetchHTTP(ctx context.Context, locator string, offset int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, locator, nil)
	if err != nil {
		return nil, fmt.Errorf("blob fetch: request %s: %w", locator, err)
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("blob fetch: get %s: %w", locator, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if offset > 0 {
			// The server ignored the Range header and is sending the whole
			// thing. Discarding the prefix keeps the caller's offset honest
			// rather than silently handing back the wrong bytes.
			if _, err := io.CopyN(io.Discard, resp.Body, offset); err != nil {
				_ = resp.Body.Close()
				return nil, fmt.Errorf("blob fetch: skip to offset %d: %w", offset, err)
			}
		}
		return resp.Body, nil
	case http.StatusPartialContent:
		return resp.Body, nil
	default:
		_ = resp.Body.Close()
		return nil, fmt.Errorf("blob fetch: %s: HTTP %d", locator, resp.StatusCode)
	}
}
