// SPDX-License-Identifier: Apache-2.0

// Package display is the client of the display server's deck routes
// (app/ktw-display, modules/slidedeck/): publishing a deck theme to its theme
// store, and exporting a deck as PowerPoint or as a standalone HTML page. The
// GUI and the agent's deck-export tool reach the same server through it.
package display

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// PublishTheme posts a resolved theme to the display server at origin,
// unless the server already holds that version, and returns the version.
func PublishTheme(ctx context.Context, client *http.Client, origin, token string, theme deckapi.ResolvedTheme) (string, error) {
	version := theme.Version()
	name := theme.Body.Name
	q := "?token=" + url.QueryEscape(token)

	// Already there (another pane, or before a GUI restart): nothing to send.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/widget/deck/theme/"+url.PathEscape(name)+"/theme.json"+q, nil)
	if err != nil {
		return "", err
	}
	if resp, err := client.Do(req); err == nil {
		var held struct {
			Version string `json:"version"`
		}
		ok := resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&held) == nil && held.Version == version
		_ = resp.Body.Close()
		if ok {
			return version, nil
		}
	}

	// []byte marshals as base64 — the store's wire form for the files.
	body, err := json.Marshal(struct {
		Name    string            `json:"name"`
		Version string            `json:"version"`
		Body    deckapi.ThemeBody `json:"body"`
		Assets  map[string][]byte `json:"assets"`
	}{name, version, theme.Body, theme.Assets})
	if err != nil {
		return "", err
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, origin+"/widget/deck/theme"+q, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("deck theme store: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return "", fmt.Errorf("deck theme store: %s %s", resp.Status, e.Error)
	}
	return version, nil
}

// Format is what a deck exports to.
type Format string

const (
	// PowerPoint of editable slides, measured from the deck page.
	FormatPptx Format = "pptx"
	// PowerPoint of one picture per slide, exactly as shown.
	FormatPptxPictures Format = "pptx-pictures"
	// One HTML page that presents offline.
	FormatHTML Format = "html"
)

// Extension is the file extension of a format's file.
func (f Format) Extension() string {
	if f == FormatHTML {
		return ".html"
	}
	return ".pptx"
}

// Valid reports whether f is one of the formats.
func (f Format) Valid() bool {
	return f == FormatPptx || f == FormatPptxPictures || f == FormatHTML
}

// Export asks the display server at origin for a deck's export and returns
// its bytes. The deck's theme, when it names one, must be published first
// (PublishTheme), and its files beside it (PublishAssets, whose reference is
// assets; nil when it has none). A PowerPoint takes the server a few seconds
// per slide.
func Export(ctx context.Context, client *http.Client, origin, token, markdown string, format Format, assets *AssetsRef) ([]byte, error) {
	if !format.Valid() {
		return nil, fmt.Errorf("deck export: unknown format %q (pptx, pptx-pictures, html)", format)
	}
	route, in := "/widget/deck/export/pptx", map[string]any{"markdown": markdown, "editable": format == FormatPptx}
	if format == FormatHTML {
		route, in = "/widget/deck/export/html", map[string]any{"markdown": markdown}
	}
	if assets != nil {
		in["assets"] = assets
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+route+"?token="+url.QueryEscape(token), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deck export: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("deck export: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(out, &e)
		return nil, fmt.Errorf("deck export: %s %s", resp.Status, strings.TrimSpace(e.Error))
	}
	return out, nil
}

// Origin splits a display server URL carrying its token (a widget URL) into
// the server's origin and the token.
func Origin(widgetURL string) (origin, token string, err error) {
	u, err := url.Parse(widgetURL)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("display server url %q: not a url", widgetURL)
	}
	return u.Scheme + "://" + u.Host, u.Query().Get("token"), nil
}

// AssetsRef names the files beside a deck in the display server's deck-assets
// store: the deck page reads them by it.
type AssetsRef struct {
	Scope   string `json:"scope"`
	Version string `json:"version"`
}

// AssetsScope names the folder of a deck — its object, or its file — as the
// display server keys it.
func AssetsScope(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:32]
}

// PublishAssets posts the files beside a deck (path relative to the deck →
// bytes) to the display server under scope, and returns where the deck page
// finds them. The version is the files' digest: the same files, the same
// version.
func PublishAssets(ctx context.Context, client *http.Client, origin, token, scope string, files map[string][]byte) (AssetsRef, error) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write(files[p])
	}
	ref := AssetsRef{Scope: scope, Version: hex.EncodeToString(h.Sum(nil))[:32]}
	// []byte marshals as base64: the store's wire form.
	body, err := json.Marshal(struct {
		AssetsRef
		Files map[string][]byte `json:"files"`
	}{ref, files})
	if err != nil {
		return AssetsRef{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, origin+"/widget/deck/assets?token="+url.QueryEscape(token), bytes.NewReader(body))
	if err != nil {
		return AssetsRef{}, err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return AssetsRef{}, fmt.Errorf("deck assets: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return AssetsRef{}, fmt.Errorf("deck assets: %s %s", resp.Status, e.Error)
	}
	return ref, nil
}

// ReadBeside reads the files a deck uses beside it (deckapi.LocalAssets) with
// read, which takes a path relative to the deck's folder; a file that cannot
// be read is returned in missing, not as an error.
func ReadBeside(paths []string, read func(rel string) ([]byte, error)) (files map[string][]byte, missing []string) {
	files = map[string][]byte{}
	for _, p := range paths {
		b, err := read(p)
		if err != nil {
			missing = append(missing, p)
			continue
		}
		files[p] = b
	}
	return files, missing
}

// FormatPNG is not a format the display server makes: it is the
// pptx-pictures export, its slide pictures written one file per slide
// (SlidePictures).
const FormatPNG Format = "png"

var slideXML = regexp.MustCompile(`^ppt/slides/slide(\d+)\.xml$`)

// SlidePictures reads the slide pictures of a pptx-pictures export, in slide
// order: each slide's first picture, as the display server captured it.
func SlidePictures(pptx []byte) ([][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(pptx), int64(len(pptx)))
	if err != nil {
		return nil, fmt.Errorf("slide pictures: not a PowerPoint file: %w", err)
	}
	files := map[string]*zip.File{}
	var slides []int
	for _, f := range zr.File {
		files[f.Name] = f
		if m := slideXML.FindStringSubmatch(f.Name); m != nil {
			n, _ := strconv.Atoi(m[1])
			slides = append(slides, n)
		}
	}
	sort.Ints(slides)
	read := func(name string) ([]byte, error) {
		f, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("slide pictures: %s missing", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer func() { _ = rc.Close() }()
		return io.ReadAll(rc)
	}
	type rel struct {
		Type   string `xml:"Type,attr"`
		Target string `xml:"Target,attr"`
	}
	out := make([][]byte, 0, len(slides))
	for _, n := range slides {
		raw, err := read(fmt.Sprintf("ppt/slides/_rels/slide%d.xml.rels", n))
		if err != nil {
			return nil, err
		}
		var rels struct {
			Rels []rel `xml:"Relationship"`
		}
		if err := xml.Unmarshal(raw, &rels); err != nil {
			return nil, fmt.Errorf("slide pictures: slide %d: %w", n, err)
		}
		var pic []byte
		for _, r := range rels.Rels {
			if !strings.HasSuffix(r.Type, "/image") {
				continue
			}
			pic, err = read(path.Clean(path.Join("ppt/slides", r.Target)))
			if err != nil {
				return nil, err
			}
			break
		}
		if pic == nil {
			return nil, fmt.Errorf("slide pictures: slide %d has no picture (not a pptx-pictures export?)", n)
		}
		out = append(out, pic)
	}
	return out, nil
}
