// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/packages"
	reviewsapi "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
)

// PackageFactsImpl serves api.PackageFactsService over the project the CWM
// service serves: its root is asked of CWM once (ObjectResolve on a module
// answers it — never derived from a folder), its store kept under the cache
// dir, one per project root.
type PackageFactsImpl struct {
	mu       sync.Mutex
	store    *packages.Store
	cacheDir string // the folder the stores go under; "" = the cache dir (Env)
}

// NewPackageFacts is the service, its stores under the cache dir.
func NewPackageFacts() *PackageFactsImpl { return &PackageFactsImpl{} }

// NewPackageFactsIn is the service with its stores under dir (tests).
func NewPackageFactsIn(dir string) *PackageFactsImpl { return &PackageFactsImpl{cacheDir: dir} }

func (p *PackageFactsImpl) storeFor(ctx stdctx.Context) (*packages.Store, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.store != nil {
		return p.store, nil
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return nil, fmt.Errorf("package facts: no CWMService: %w", err)
	}
	mods, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: cwmapi.CapModule})
	if err != nil || len(mods.Objects) == 0 {
		return nil, fmt.Errorf("package facts: no module to find the project root from: %v", err)
	}
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: mods.Objects[0].ID})
	if err != nil || res.ProjectRoot == "" {
		return nil, fmt.Errorf("package facts: the project root is not known: %v", err)
	}
	sum := sha256.Sum256([]byte(res.ProjectRoot))
	base := p.cacheDir
	if base == "" {
		base = application.Env().GetCacheSubDir("gocode/packages")
	}
	dir := filepath.Join(base, hex.EncodeToString(sum[:6]))
	p.store = packages.New(res.ProjectRoot, dir)
	return p.store, nil
}

// PackageFacts implements api.PackageFactsService.
func (p *PackageFactsImpl) PackageFacts(ctx stdctx.Context, in api.PackageFactsInput) (api.PackageFactsOutput, error) {
	s, err := p.storeFor(ctx)
	if err != nil {
		return api.PackageFactsOutput{}, err
	}
	return s.PackageFacts(ctx, in)
}

// PackageList implements api.PackageFactsService.
func (p *PackageFactsImpl) PackageList(ctx stdctx.Context, in api.PackageListInput) (api.PackageListOutput, error) {
	s, err := p.storeFor(ctx)
	if err != nil {
		return api.PackageListOutput{}, err
	}
	return s.PackageList(ctx, in)
}

// PackageRecord implements api.PackageFactsService.
func (p *PackageFactsImpl) PackageRecord(ctx stdctx.Context, in api.PackageRecordInput) (api.PackageRecordOutput, error) {
	s, err := p.storeFor(ctx)
	if err != nil {
		return api.PackageRecordOutput{}, err
	}
	return s.PackageRecord(ctx, in)
}

// ChangeOwners implements api.PackageFactsService.
func (p *PackageFactsImpl) ChangeOwners(ctx stdctx.Context, in reviewsapi.ChangeOwnersInput) (reviewsapi.ChangeOwnersOutput, error) {
	s, err := p.storeFor(ctx)
	if err != nil {
		return reviewsapi.ChangeOwnersOutput{}, err
	}
	return s.ChangeOwners(ctx, in)
}
