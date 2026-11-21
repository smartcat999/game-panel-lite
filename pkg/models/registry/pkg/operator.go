package pkg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	artifactdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/artifact"
	pkgdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/pkg"
	repositorydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/repository"
	upstreamdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/upstream"
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Package], error)
	Get(ctx context.Context, name string) (*registryv1.Package, error)
	GetForGameVersion(ctx context.Context, name, gameVersion string) (*registryv1.Package, error)
	Create(ctx context.Context, in *registryv1.Package) (*registryv1.Package, error)
	Update(ctx context.Context, name string, in *registryv1.Package) (*registryv1.Package, error)
	Delete(ctx context.Context, name string) error
	FetchIcon(ctx context.Context, name, encodedURL, sig string) ([]byte, string, error)
}

type operator struct {
	db         *pkgdb.DB
	artDB      *artifactdb.DB
	repoDB     *repositorydb.DB
	upstreamDB *upstreamdb.DB
	cache      cache.Interface
}

func NewOperator(reg *repo.Registry, cacheStore cache.Interface) Operator {
	op := &operator{cache: cacheStore}
	if reg != nil {
		op.db = pkgdb.NewDB(reg.Pool())
		op.artDB = artifactdb.NewDB(reg.Pool())
		op.repoDB = repositorydb.NewDB(reg.Pool())
		op.upstreamDB = upstreamdb.NewDB(reg.Pool())
	}
	return op
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*registryv1.Package], error) {
	repoName := strings.TrimSpace(opts.FieldSelector["repository"])
	cachedOnly := strings.EqualFold(strings.TrimSpace(opts.FieldSelector["cached"]), "true")
	refreshUpstream := strings.EqualFold(strings.TrimSpace(opts.FieldSelector["refresh"]), "true")
	var repoObj *registryv1.Repository
	if repoName != "" {
		var err error
		repoObj, err = o.repoDB.Get(ctx, repoName)
		if err != nil {
			return nil, err
		}
		if !cachedOnly && repoObj.Spec.Mode == registryv1.RepositoryModeProxy {
			if list, handled, pErr := o.listProxyCatalog(ctx, repoObj, opts); handled {
				return list, pErr
			}
		}
	}
	cleanOpts := stripCatalogSelectors(opts)
	list, err := o.db.List(ctx, cleanOpts)
	if err != nil || list == nil {
		return list, err
	}
	if repoObj != nil {
		o.normalizeRepoPackageList(ctx, repoObj, list, cachedOnly, refreshUpstream)
	} else if o.repoDB != nil {
		o.normalizeClusterPackageList(ctx, list, cachedOnly, refreshUpstream)
	}
	return list, nil
}

func stripCatalogSelectors(opts db.ListOptions) db.ListOptions {
	cleanOpts := opts
	if len(opts.FieldSelector) == 0 {
		return cleanOpts
	}
	cleanSel := make(map[string]string, len(opts.FieldSelector))
	for k, v := range opts.FieldSelector {
		if k == "sort" || k == "gameVersion" || k == "resolve" || k == "cached" || k == "refresh" {
			continue
		}
		cleanSel[k] = v
	}
	cleanOpts.FieldSelector = cleanSel
	return cleanOpts
}

func (o *operator) Get(ctx context.Context, name string) (*registryv1.Package, error) {
	return o.GetForGameVersion(ctx, name, "")
}

func (o *operator) GetForGameVersion(ctx context.Context, name, gameVersion string) (*registryv1.Package, error) {
	gv := strings.TrimSpace(gameVersion)
	persisted, err := o.db.Get(ctx, name)
	if err == nil {
		return o.enrichPersistedPackage(ctx, persisted, gv), nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return nil, err
	}
	repoName, pkgID, parseErr := registryv1.ParsePackageName(name)
	if parseErr != nil {
		return nil, db.ErrNotFound
	}
	repoObj, rErr := o.repoDB.Get(ctx, repoName)
	if rErr != nil || repoObj.Spec.Mode != registryv1.RepositoryModeProxy {
		return nil, db.ErrNotFound
	}
	if repoObj.Spec.Format == registryv1.FormatServerPackage {
		return o.detailServerProxyPackage(ctx, repoObj, pkgID)
	}
	return o.detailProxyPackage(ctx, repoObj, pkgID, gv)
}

func (o *operator) enrichPersistedPackage(ctx context.Context, persisted *registryv1.Package, gameVersion string) *registryv1.Package {
	if persisted == nil || o.repoDB == nil {
		return persisted
	}
	repoObj, rErr := o.repoDB.Get(ctx, persisted.Repository)
	if rErr != nil || repoObj.Spec.Mode != registryv1.RepositoryModeProxy {
		return persisted
	}
	if repoObj.Spec.Format == registryv1.FormatServerPackage {
		return o.enrichServerProxyPackage(ctx, repoObj, persisted)
	}
	if gameVersion != "" || repoObj.Spec.Format == registryv1.FormatThunderstoreMod {
		if live := o.overlayGameVersionProxyPackage(ctx, repoObj, persisted, gameVersion); live != nil {
			return o.signPackageIcon(ctx, repoObj, live)
		}
	}
	if isStubPackage(persisted) {
		persisted = o.enrichPersistedStub(ctx, repoObj, persisted)
	}
	return o.signPackageIcon(ctx, repoObj, persisted)
}

func (o *operator) overlayGameVersionProxyPackage(ctx context.Context, repoObj *registryv1.Repository, persisted *registryv1.Package, gameVersion string) *registryv1.Package {
	detailed, err := o.detailProxyPackage(ctx, repoObj, persisted.Package, gameVersion)
	if err != nil || detailed == nil {
		return nil
	}
	return mergePersistedPackageForDetail(persisted, detailed)
}

func mergePersistedPackageForDetail(persisted, detailed *registryv1.Package) *registryv1.Package {
	if detailed == nil {
		return persisted
	}
	if persisted == nil {
		return detailed
	}
	merged := *detailed
	merged.Metadata = persisted.Metadata
	merged.Repository = persisted.Repository
	merged.Package = persisted.Package
	// Preserve live gameVersion-scoped LatestVersion, AvailableVersions, and Dependencies instead of persisted global fields.
	mergePackageLocalMeta(&merged, persisted)
	return &merged
}

func isStubPackage(p *registryv1.Package) bool {
	if p == nil {
		return false
	}
	title := strings.TrimSpace(p.Title)
	return (title == "" || title == p.Package) && strings.TrimSpace(p.IconURL) == "" && strings.TrimSpace(p.Description) == ""
}

func (o *operator) enrichPersistedStub(ctx context.Context, repoObj *registryv1.Repository, persisted *registryv1.Package) *registryv1.Package {
	detailed, err := o.detailProxyPackage(ctx, repoObj, persisted.Package, "")
	if err != nil || detailed == nil {
		return persisted
	}
	merged := mergePersistedPackageForDetail(persisted, detailed)
	if merged.LatestVersion == "" {
		merged.LatestVersion = persisted.LatestVersion
	}
	return merged
}

func (o *operator) Create(ctx context.Context, in *registryv1.Package) (*registryv1.Package, error) {
	cp := *in
	cp.DefaultMeta()
	if _, err := o.repoDB.Get(ctx, cp.Repository); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return nil, fmt.Errorf("%w: repository %q not found", models.ErrBadRequest, cp.Repository)
		}
		return nil, err
	}
	created, err := o.db.Create(ctx, &cp)
	if err != nil {
		if errors.Is(err, db.ErrAlreadyExists) || errors.Is(err, db.ErrConflict) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	return created, nil
}

func (o *operator) Update(ctx context.Context, name string, in *registryv1.Package) (*registryv1.Package, error) {
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	cp := *in
	cp.Metadata.Name = existing.Metadata.Name
	cp.Repository = existing.Repository
	cp.Package = existing.Package
	updated, err := o.db.Update(ctx, &cp)
	if err != nil {
		if errors.Is(err, db.ErrConflict) || errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	return updated, nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	return o.db.Delete(ctx, name)
}
