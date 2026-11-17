// SPDX-License-Identifier: Apache-2.0

// Package catalog fetches, caches, and parses marketplace catalogs
// (katyoti-marketplace.yaml). Supports http(s) URLs and local file paths;
// falls back to the on-disk cache when a live fetch fails (offline tolerance).
package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/signing"
)

// Fetch downloads + parses a marketplace catalog, caching the raw bytes under
// CacheDir/marketplace/<name>.yaml. For a `trust: pinned` marketplace the
// catalog's `<url>.minisig` signature MUST verify against the pinned public key
// BEFORE the catalog is parsed/used — a bad or missing signature is a hard
// refuse (team-marketplace.md §2). `trust: none` skips verification (the
// per-asset sha256 gate at install still applies).
func Fetch(ctx context.Context, m mktapi.Marketplace) (mktapi.Catalog, error) {
	data, ferr := fetchBytes(ctx, m.URL)
	if ferr != nil {
		if cached, cerr := readCache(m.Name); cerr == nil {
			data = cached
		} else {
			return mktapi.Catalog{}, ferr
		}
	} else {
		_ = writeCache(m.Name, data)
	}
	if err := verifyTrust(ctx, m, data); err != nil {
		return mktapi.Catalog{}, err
	}
	var cat mktapi.Catalog
	if err := config.DecodeBytes(data, marshal.YAML, &cat); err != nil {
		return mktapi.Catalog{}, fmt.Errorf("parse catalog %q: %w", m.Name, err)
	}
	return ForChannel(cat, m.Channel), nil
}

// ForChannel is a catalog as an installation following a channel sees it:
// stable (the default) never sees a canary version - a release candidate -;
// canary sees every version.
func ForChannel(cat mktapi.Catalog, channel string) mktapi.Catalog {
	if channel == mktapi.ChannelCanary {
		return cat
	}
	items := make([]mktapi.Item, 0, len(cat.Items))
	for _, it := range cat.Items {
		var vs []mktapi.ItemVersion
		for _, v := range it.Versions {
			if v.Channel != mktapi.ChannelCanary {
				vs = append(vs, v)
			}
		}
		if len(it.Versions) > 0 && len(vs) == 0 {
			continue // only candidates: not there for a stable installation
		}
		it.Versions = vs
		items = append(items, it)
	}
	cat.Items = items
	return cat
}

// verifyTrust enforces the install-time provenance gate for a pinned
// marketplace: fetch `<url>.minisig` and verify it over the index bytes against
// the pinned key. Fails closed — no key, no signature, or a bad signature all
// refuse. A `trust: none` (or empty) marketplace is a no-op.
func verifyTrust(ctx context.Context, m mktapi.Marketplace, index []byte) error {
	if m.Trust != mktapi.TrustPinned {
		return nil
	}
	if strings.TrimSpace(m.PubKey) == "" {
		return fmt.Errorf("marketplace %q is trust=pinned but has no pinned public key — re-add with `--pubkey <file>`", m.Name)
	}
	pk, err := signing.ParsePublicKey([]byte(m.PubKey))
	if err != nil {
		return fmt.Errorf("marketplace %q: invalid pinned public key: %w", m.Name, err)
	}
	sig, serr := fetchBytes(ctx, m.URL+".minisig")
	if serr != nil {
		if cached, cerr := readSigCache(m.Name); cerr == nil {
			sig = cached
		} else {
			return fmt.Errorf("marketplace %q: cannot fetch signature %s.minisig: %w", m.Name, m.URL, serr)
		}
	} else {
		_ = writeSigCache(m.Name, sig)
	}
	if err := signing.Verify(index, sig, pk); err != nil {
		return fmt.Errorf("marketplace %q: catalog signature did NOT verify — refusing to trust it: %w", m.Name, err)
	}
	return nil
}

func fetchBytes(ctx context.Context, url string) ([]byte, error) {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", url, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	// local path (supports file:// and bare paths)
	return os.ReadFile(strings.TrimPrefix(url, "file://"))
}

func cacheDir() string {
	return application.Env().GetCacheSubDir("marketplace")
}

func writeCache(name string, data []byte) error {
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".yaml"), data, 0o644)
}

func readCache(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(cacheDir(), name+".yaml"))
}

func writeSigCache(name string, data []byte) error {
	dir := cacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".yaml.minisig"), data, 0o644)
}

func readSigCache(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(cacheDir(), name+".yaml.minisig"))
}
