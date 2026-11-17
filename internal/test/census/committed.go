// SPDX-License-Identifier: Apache-2.0

package census

import (
	"bytes"
	"context"
	"unicode/utf8"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// committed.go — the census hashes what is committed, not the checkout's line
// endings.
//
// A binary embeds its files as the working copy holds them. On Windows git
// checks text files out with CRLF (core.autocrlf, or the repository's
// `eol=lf` files left CRLF by an older checkout) while the commit — and every
// clean checkout the gate tests on — holds LF. Hashing the embedded bytes as
// they are made a bless in the working copy and a bless on a clean checkout
// disagree on every text layer that crossed a CRLF file, so the golden could
// never pass in both places.
//
// The census therefore records, for a text layer carried with CRLF, the hash
// of the same bytes with LF line endings — the hash a clean checkout produces —
// and an immutable provider's epoch recomputed over those hashes. What the
// binary itself publishes is unchanged: this is how the GUARD reads it.

// committedHashes maps hashes of the working copy's bytes to the hashes of
// what is committed. The zero value maps nothing.
type committedHashes struct {
	layer  map[string]string // a CRLF text layer's hash → its LF hash
	digest map[string]string // a provider's epoch → the epoch over LF hashes
}

func (c committedHashes) layerHash(h string) string {
	if n, ok := c.layer[h]; ok {
		return n
	}
	return h
}

func (c committedHashes) providerDigest(d string) string {
	if n, ok := c.digest[d]; ok {
		return n
	}
	return d
}

// committedOf reads the bytes of every layer the binary carries — the
// providers' own enumeration (where every layer below the bulk band still has
// its bytes) and the built manifest's resident layers — and maps each CRLF text
// layer's hash to its LF hash, then each immutable provider's epoch to the
// epoch over the LF hashes. Bulk layers carry no bytes and keep their hash.
func committedOf(app *application.Application, m *mbapi.Manifest) committedHashes {
	ctx := application.WithApplication(context.Background(), app)
	return committedFrom(ctx, application.UseAppExtensions[contribapi.ContributionProvider](app), m)
}

// committedFrom is committedOf over the providers given.
func committedFrom(ctx context.Context, providers []contribapi.ContributionProvider, m *mbapi.Manifest) committedHashes {
	c := committedHashes{layer: map[string]string{}, digest: map[string]string{}}
	note := func(l contribapi.LayerRef) {
		if n, ok := lfHash(l.Inline); ok && n != l.Hash && l.Hash != "" {
			c.layer[l.Hash] = n
		}
	}
	// A layer derived from a CRLF source carries its line endings encoded: a
	// frontmatter kind folds the file's body into its data as a JSON string,
	// where CRLF reads `\r\n`. A clean checkout derives `\n` there, so that
	// layer is remapped too - only within an entry whose own source is CRLF
	// text, so no other JSON's escaped CR is touched.
	noteEntry := func(e contribapi.Contribution) {
		crlf := false
		for _, l := range e.Layers {
			if _, ok := lfHash(l.Inline); ok {
				crlf = true
			}
			note(l)
		}
		if !crlf {
			return
		}
		for _, l := range e.Layers {
			if n, ok := lfEscapedHash(l.Inline); ok && n != l.Hash && l.Hash != "" {
				c.layer[l.Hash] = n
			}
		}
	}
	for _, e := range m.Contributions {
		noteEntry(e)
	}
	var enumerations [][]contribapi.Contribution
	for _, p := range providers {
		items, err := p.Enumerate(ctx)
		if err != nil {
			continue
		}
		for _, e := range items {
			noteEntry(e)
		}
		enumerations = append(enumerations, items)
	}
	if len(c.layer) == 0 {
		return c
	}
	for _, items := range enumerations {
		orig := contribapi.DigestOf(items)
		if norm := contribapi.DigestOf(c.remapped(items)); norm != orig {
			c.digest[orig] = norm
		}
	}
	return c
}

// remapped returns items with every layer hash read through c, leaving items
// itself untouched (Layers is a map a struct copy shares).
func (c committedHashes) remapped(items []contribapi.Contribution) []contribapi.Contribution {
	out := make([]contribapi.Contribution, len(items))
	for i, e := range items {
		layers := make(map[string]contribapi.LayerRef, len(e.Layers))
		for name, l := range e.Layers {
			l.Hash = c.layerHash(l.Hash)
			layers[name] = l
		}
		e.Layers = layers
		out[i] = e
	}
	return out
}

// lfHash returns the hash of b with CRLF line endings turned to LF, and true,
// when b is text that holds a CRLF; false otherwise (no bytes, binary content —
// an image's bytes may hold "\r\n" that are not line endings — or LF already).
func lfHash(b []byte) (string, bool) {
	if !bytes.Contains(b, []byte("\r\n")) || bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
		return "", false
	}
	return contribapi.HashContent(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))), true
}

// lfEscapedHash returns the hash of b with every JSON-escaped CRLF (`\r\n`, its
// backslash not itself escaped) turned to an escaped LF (`\n`), and true, when b
// is text holding one; false otherwise.
func lfEscapedHash(b []byte) (string, bool) {
	esc := []byte(`\r\n`)
	if !bytes.Contains(b, esc) || bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
		return "", false
	}
	out := make([]byte, 0, len(b))
	changed := false
	for i := 0; i < len(b); {
		if b[i] != '\\' {
			out = append(out, b[i])
			i++
			continue
		}
		// An escape: `\r\n` becomes `\n`; any other keeps its two bytes, so an
		// escaped backslash never starts a false match.
		if bytes.HasPrefix(b[i:], esc) {
			out = append(out, '\\', 'n')
			i += len(esc)
			changed = true
			continue
		}
		out = append(out, b[i:min(i+2, len(b))]...)
		i += 2
	}
	if !changed {
		return "", false
	}
	return contribapi.HashContent(out), true
}
