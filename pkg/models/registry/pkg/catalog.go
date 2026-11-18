package pkg

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
)

const upstreamRefreshTimeout = 4 * time.Second

func (o *operator) mirrorForUpstream(up *registryv1.Upstream, repoObj *registryv1.Repository) (*addonmirror.Service, addonmirror.Source, bool) {
	if up == nil || repoObj == nil {
		return nil, addonmirror.Source{}, false
	}
	prefix := "/apis/registry.gamepanel.io/v1/packages/" + registryv1.FormatPackageName(repoObj.Metadata.Name, "catalog") + "/icon"
	svc, src := addonmirror.NewForRegistry(o.cache, config.Get().Registry, addonmirror.RegistryParams{
		Provider:            up.Provider,
		Endpoint:            up.Endpoint,
		Format:              repoObj.Spec.Format,
		ProxyURL:            up.ProxyURL,
		APIKey:              up.APIKey,
		SteamAppID:          up.SteamAppID,
		ModrinthLoader:      up.ModrinthLoader,
		ModrinthProjectType: up.ModrinthProjectType,
		ImageProxyPrefix:    prefix,
	})
	if !svc.SupportsCatalog(src) {
		return nil, addonmirror.Source{}, false
	}
	return svc, src, true
}

func (o *operator) listProxyCatalog(ctx context.Context, repoObj *registryv1.Repository, opts db.ListOptions) (*metav1.List[*registryv1.Package], bool, error) {
	up, err := o.upstreamDB.Get(ctx, repoObj.Spec.UpstreamName)
	if err != nil {
		return nil, false, nil
	}
	svc, src, ok := o.mirrorForUpstream(up, repoObj)
	if !ok {
		return nil, false, nil
	}
	resolveRaw := strings.TrimSpace(opts.FieldSelector["resolve"])
	gameVer := strings.TrimSpace(opts.FieldSelector["gameVersion"])
	var res *addonmirror.Result
	if resolveRaw != "" {
		res, err = svc.Resolve(ctx, src, resolveRaw, gameVer)
	} else {
		limit := opts.Limit
		if limit <= 0 || limit > 30 {
			limit = 20
		}
		page := 1
		if opts.Offset > 0 && limit > 0 {
			page = (opts.Offset / limit) + 1
		}
		res, err = svc.Search(ctx, src, addonmirror.SearchParams{
			Query:       opts.FieldSelector["q"],
			Sort:        opts.FieldSelector["sort"],
			GameVersion: gameVer,
			Page:        page,
			Limit:       limit,
		})
	}
	if err != nil {
		return nil, true, fmt.Errorf("%w: %v", models.ErrBadGateway, err)
	}
	items := make([]*registryv1.Package, 0, len(res.Items))
	for _, it := range res.Items {
		items = append(items, convertMirrorItem(repoObj.Metadata.Name, it))
	}
	o.overlayPersistedPackageMeta(ctx, repoObj.Metadata.Name, items)
	return &metav1.List[*registryv1.Package]{
		TypeMeta: metav1.TypeMeta{APIVersion: registryv1.GroupVersion, Kind: "PackageList"},
		Metadata: metav1.ListMeta{TotalCount: res.Total},
		Items:    items,
	}, true, nil
}

func (o *operator) detailProxyPackage(ctx context.Context, repoObj *registryv1.Repository, pkgID string) (*registryv1.Package, error) {
	up, err := o.upstreamDB.Get(ctx, repoObj.Spec.UpstreamName)
	if err != nil {
		return nil, db.ErrNotFound
	}
	svc, src, ok := o.mirrorForUpstream(up, repoObj)
	if !ok {
		return nil, db.ErrNotFound
	}
	it, err := svc.Detail(ctx, src, pkgID, "")
	if err != nil {
		if errors.Is(err, addonmirror.ErrNotFound) {
			return nil, db.ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", models.ErrBadGateway, err)
	}
	pkg := convertMirrorItem(repoObj.Metadata.Name, *it)
	o.overlayPersistedPackageMeta(ctx, repoObj.Metadata.Name, []*registryv1.Package{pkg})
	return pkg, nil
}

func (o *operator) overlayPersistedPackageMeta(ctx context.Context, repoName string, items []*registryv1.Package) {
	if o.db == nil || len(items) == 0 {
		return
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil && item.Package != "" {
			names = append(names, registryv1.FormatPackageName(repoName, item.Package))
		}
	}
	if len(names) == 0 {
		return
	}
	stored, err := o.db.List(ctx, db.ListOptions{
		FieldSelector:  map[string]string{"metadata.name.in": strings.Join(names, ",")},
		Limit:          len(names),
		SkipTotalCount: true,
	})
	if err != nil || stored == nil || len(stored.Items) == 0 {
		return
	}
	byName := make(map[string]*registryv1.Package, len(stored.Items))
	for _, s := range stored.Items {
		if s != nil {
			byName[s.Metadata.Name] = s
		}
	}
	for _, item := range items {
		if item != nil {
			mergePackageLocalMeta(item, byName[registryv1.FormatPackageName(repoName, item.Package)])
		}
	}
}

func mergePackageLocalMeta(dst, src *registryv1.Package) {
	if dst == nil || src == nil {
		return
	}
	if dst.SizeBytes <= 0 && src.SizeBytes > 0 {
		dst.SizeBytes = src.SizeBytes
	}
	if len(dst.ModIDs) == 0 && len(src.ModIDs) > 0 {
		dst.ModIDs = append([]string(nil), src.ModIDs...)
	}
	if len(dst.MapFolders) == 0 && len(src.MapFolders) > 0 {
		dst.MapFolders = append([]string(nil), src.MapFolders...)
	}
	if len(dst.ConfigSchema) == 0 && len(src.ConfigSchema) > 0 {
		dst.ConfigSchema = append([]registryv1.AddonConfigField(nil), src.ConfigSchema...)
	}
}

func (o *operator) FetchIcon(ctx context.Context, name, encodedURL, sig string) ([]byte, string, error) {
	repoName, _, err := registryv1.ParsePackageName(name)
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid package name", models.ErrBadRequest)
	}
	var up *registryv1.Upstream
	if repoObj, rErr := o.repoDB.Get(ctx, repoName); rErr == nil && repoObj.Spec.UpstreamName != "" {
		up, _ = o.upstreamDB.Get(ctx, repoObj.Spec.UpstreamName)
	}
	cfg := config.Get().Registry
	proxyURL := strings.TrimSpace(cfg.UpstreamProxyURL)
	if up != nil && strings.TrimSpace(up.ProxyURL) != "" {
		proxyURL = strings.TrimSpace(up.ProxyURL)
	}
	svc := addonmirror.New(o.cache, addonmirror.Options{
		UpstreamProxyURL: proxyURL,
		CacheTTL:         cfg.CatalogCacheTTL,
		Timeout:          cfg.Timeout,
	})
	data, ctype, fErr := svc.FetchImage(ctx, encodedURL, sig)
	if fErr != nil {
		if errors.Is(fErr, addonmirror.ErrInvalidRequest) {
			return nil, "", fmt.Errorf("%w: %v", models.ErrBadRequest, fErr)
		}
		if errors.Is(fErr, addonmirror.ErrInvalidSignature) || errors.Is(fErr, addonmirror.ErrForbiddenImageHost) {
			return nil, "", fmt.Errorf("%w: %v", models.ErrForbidden, fErr)
		}
		return nil, "", fmt.Errorf("%w: %v", models.ErrBadGateway, fErr)
	}
	return data, ctype, nil
}

func convertMirrorItem(repoName string, it addonmirror.Item) *registryv1.Package {
	deps := make([]registryv1.PackageDependency, 0, len(it.Dependencies))
	for _, d := range it.Dependencies {
		deps = append(deps, registryv1.PackageDependency{Package: d.ID, Title: d.Title})
	}
	p := &registryv1.Package{
		Repository:    repoName,
		Package:       it.ID,
		Title:         it.Title,
		Summary:       it.Summary,
		Description:   it.Description,
		Author:        it.Author,
		IconURL:       it.IconURL,
		GalleryURLs:   it.GalleryURLs,
		SourceURL:     it.SourceURL,
		Downloads:     it.Downloads,
		SizeBytes:     it.SizeBytes,
		LatestVersion: it.LatestVersion,
		UpdatedAt:     it.UpdatedAt,
		Tags:          it.Tags,
		ModIDs:        it.ModIDs,
		MapFolders:    it.MapFolders,
		Dependencies:  deps,
	}
	p.DefaultMeta()
	return p
}

func (o *operator) newServerMirrorService(up *registryv1.Upstream) *addonmirror.Service {
	var params addonmirror.RegistryParams
	if up != nil {
		params = addonmirror.RegistryParams{
			Provider: up.Provider,
			Endpoint: up.Endpoint,
			ProxyURL: up.ProxyURL,
			APIKey:   up.APIKey,
		}
	}
	svc, _ := addonmirror.NewForRegistry(o.cache, config.Get().Registry, params)
	return svc
}

func (o *operator) listUpstreamServerVersions(ctx context.Context, repoObj *registryv1.Repository, pkgID string) []string {
	if repoObj == nil || o.upstreamDB == nil || strings.TrimSpace(repoObj.Spec.UpstreamName) == "" {
		return nil
	}
	up, err := o.upstreamDB.Get(ctx, repoObj.Spec.UpstreamName)
	if err != nil {
		return nil
	}
	svc := o.newServerMirrorService(up)
	vers, err := svc.ListServerVersions(ctx, up.Provider, up.Endpoint, pkgID)
	if err != nil {
		return nil
	}
	return vers
}

func (o *operator) enrichServerProxyPackage(
	ctx context.Context,
	repoObj *registryv1.Repository,
	persisted *registryv1.Package,
) *registryv1.Package {
	if persisted == nil {
		return nil
	}
	cp := *persisted
	vers := o.listUpstreamServerVersions(ctx, repoObj, persisted.Package)
	if len(vers) > 0 {
		cp.AvailableVersions = vers
		if addonmirror.IsLatestAlias(cp.LatestVersion) {
			cp.LatestVersion = vers[0]
		}
	}
	return &cp
}

func (o *operator) detailServerProxyPackage(
	ctx context.Context,
	repoObj *registryv1.Repository,
	pkgID string,
) (*registryv1.Package, error) {
	if repoObj == nil || strings.TrimSpace(repoObj.Spec.UpstreamName) == "" {
		return nil, db.ErrNotFound
	}
	up, err := o.upstreamDB.Get(ctx, repoObj.Spec.UpstreamName)
	if err != nil {
		return nil, db.ErrNotFound
	}
	title := strings.TrimSpace(repoObj.Spec.DisplayName)
	if title == "" {
		title = pkgID
	}
	p := &registryv1.Package{
		Repository: repoObj.Metadata.Name,
		Package:    pkgID,
		Title:      title,
	}
	p.DefaultMeta()
	if vers, vErr := o.newServerMirrorService(up).ListServerVersions(ctx, up.Provider, up.Endpoint, pkgID); vErr == nil && len(vers) > 0 {
		p.AvailableVersions = vers
		p.LatestVersion = vers[0]
	}
	return p, nil
}

func (o *operator) backfillCachedPackages(
	ctx context.Context,
	repoObj *registryv1.Repository,
	list *metav1.List[*registryv1.Package],
) {
	if o.artDB == nil || repoObj == nil || list == nil {
		return
	}
	artList, err := o.artDB.List(ctx, db.ListOptions{
		FieldSelector: map[string]string{"repository": repoObj.Metadata.Name},
		Limit:         500,
	})
	if err != nil || artList == nil || len(artList.Items) == 0 {
		return
	}
	present := make(map[string]bool, len(list.Items))
	for _, p := range list.Items {
		if p != nil && p.Repository == repoObj.Metadata.Name && p.Package != "" {
			present[p.Package] = true
		}
	}
	for _, art := range artList.Items {
		if art == nil || art.Metadata.DeletionTimestamp != nil || art.Spec.Package == "" || present[art.Spec.Package] {
			continue
		}
		present[art.Spec.Package] = true
		if pkg := o.healMissingCachedPackage(ctx, repoObj, art); pkg != nil {
			list.Items = append(list.Items, pkg)
			list.Metadata.TotalCount++
		}
	}
}

func (o *operator) healMissingCachedPackage(
	ctx context.Context,
	repoObj *registryv1.Repository,
	art *registryv1.Artifact,
) *registryv1.Package {
	coord := registryv1.FormatPackageName(repoObj.Metadata.Name, art.Spec.Package)
	if existing, gErr := o.db.Get(ctx, coord); gErr == nil && existing != nil {
		return nil
	}
	var (
		detailed *registryv1.Package
		err      error
	)
	if repoObj.Spec.Format == registryv1.FormatServerPackage {
		detailed, err = o.detailServerProxyPackage(ctx, repoObj, art.Spec.Package)
	} else {
		detailed, err = o.detailProxyPackage(ctx, repoObj, art.Spec.Package)
	}
	if err != nil || detailed == nil {
		log.S().Warnw("failed to backfill missing cached package metadata", "package", coord, "error", err)
		return nil
	}
	if detailed.LatestVersion == "" {
		detailed.LatestVersion = art.Spec.Version
	}
	if detailed.SizeBytes <= 0 && art.Status.Descriptor != nil && art.Status.Descriptor.SizeBytes > 0 {
		detailed.SizeBytes = art.Status.Descriptor.SizeBytes
	}
	return detailed
}

func (o *operator) normalizeProxyPackageItems(
	ctx context.Context,
	repoObj *registryv1.Repository,
	items []*registryv1.Package,
	refreshLatest bool,
) {
	if len(items) == 0 || repoObj == nil {
		return
	}
	svc, src, ok := o.proxyMirrorForRepo(ctx, repoObj)
	var latestMap map[string]string
	if refreshLatest && ok {
		latestMap = o.resolveRepoLatestVersions(ctx, svc, src, items)
	}
	for i, item := range items {
		if item == nil {
			continue
		}
		if isStubPackage(item) {
			item = o.enrichPersistedStub(ctx, repoObj, item)
		}
		if liveVer := latestMap[item.Package]; liveVer != "" && !addonmirror.IsLatestAlias(liveVer) && item.LatestVersion != liveVer {
			item = o.applyLiveLatestVersion(ctx, item, liveVer)
		}
		if ok {
			item = signPackageIconWithService(svc, src.ImageProxyPrefix, item)
		}
		items[i] = item
	}
}

func (o *operator) resolveRepoLatestVersions(
	ctx context.Context,
	svc *addonmirror.Service,
	src addonmirror.Source,
	items []*registryv1.Package,
) map[string]string {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		if it != nil && it.Package != "" {
			ids = append(ids, it.Package)
		}
	}
	out, err := svc.ResolveLatestVersions(ctx, src, ids)
	if err != nil {
		return map[string]string{}
	}
	return out
}

func (o *operator) applyLiveLatestVersion(_ context.Context, item *registryv1.Package, liveVer string) *registryv1.Package {
	cp := *item
	cp.LatestVersion = liveVer
	return &cp
}

func (o *operator) signPackageIcon(
	ctx context.Context,
	repoObj *registryv1.Repository,
	pkg *registryv1.Package,
) *registryv1.Package {
	if pkg == nil || !hasExternalHTTPURL(pkg.IconURL) {
		return pkg
	}
	svc, src, ok := o.proxyMirrorForRepo(ctx, repoObj)
	if !ok {
		return pkg
	}
	return signPackageIconWithService(svc, src.ImageProxyPrefix, pkg)
}

func (o *operator) proxyMirrorForRepo(
	ctx context.Context,
	repoObj *registryv1.Repository,
) (*addonmirror.Service, addonmirror.Source, bool) {
	if o.upstreamDB == nil || repoObj == nil || strings.TrimSpace(repoObj.Spec.UpstreamName) == "" {
		return nil, addonmirror.Source{}, false
	}
	up, err := o.upstreamDB.Get(ctx, repoObj.Spec.UpstreamName)
	if err != nil {
		return nil, addonmirror.Source{}, false
	}
	return o.mirrorForUpstream(up, repoObj)
}

func signPackageIconWithService(
	svc *addonmirror.Service,
	prefix string,
	pkg *registryv1.Package,
) *registryv1.Package {
	if pkg == nil || svc == nil || prefix == "" || !hasExternalHTTPURL(pkg.IconURL) {
		return pkg
	}
	signed := svc.SignImageProxyURL(prefix, pkg.IconURL)
	if signed == "" || signed == pkg.IconURL {
		return pkg
	}
	cp := *pkg
	cp.IconURL = signed
	return &cp
}

func hasExternalHTTPURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://")
}

func (o *operator) normalizeRepoPackageList(
	ctx context.Context,
	repoObj *registryv1.Repository,
	list *metav1.List[*registryv1.Package],
	cachedOnly, refreshUpstream bool,
) {
	if repoObj == nil || repoObj.Spec.Mode != registryv1.RepositoryModeProxy || list == nil {
		return
	}
	if cachedOnly && !refreshUpstream {
		o.synthesizeLocalCachedPackages(ctx, repoObj, list)
		o.signRepoPackageIcons(ctx, repoObj, list.Items)
		return
	}
	if repoObj.Spec.Format == registryv1.FormatServerPackage {
		if cachedOnly {
			o.backfillCachedPackages(ctx, repoObj, list)
			o.enrichServerItemsForRepo(ctx, repoObj, list.Items)
		}
		return
	}
	if cachedOnly {
		o.backfillCachedPackages(ctx, repoObj, list)
	}
	o.normalizeProxyPackageItems(ctx, repoObj, list.Items, refreshUpstream)
}

func (o *operator) synthesizeLocalCachedPackages(
	ctx context.Context,
	repoObj *registryv1.Repository,
	list *metav1.List[*registryv1.Package],
) {
	if o.artDB == nil || repoObj == nil || list == nil {
		return
	}
	artList, err := o.artDB.List(ctx, db.ListOptions{
		FieldSelector: map[string]string{"repository": repoObj.Metadata.Name},
		Limit:         500,
	})
	if err != nil || artList == nil {
		return
	}
	present := make(map[string]bool, len(list.Items))
	for _, p := range list.Items {
		if p != nil && p.Repository == repoObj.Metadata.Name && p.Package != "" {
			present[p.Package] = true
		}
	}
	for _, art := range artList.Items {
		if art == nil || art.Metadata.DeletionTimestamp != nil || art.Spec.Package == "" || present[art.Spec.Package] {
			continue
		}
		present[art.Spec.Package] = true
		list.Items = append(list.Items, localStubPackageFromArtifact(repoObj, art))
		list.Metadata.TotalCount++
	}
}

func localStubPackageFromArtifact(repoObj *registryv1.Repository, art *registryv1.Artifact) *registryv1.Package {
	title := strings.TrimSpace(art.Spec.Package)
	if repoObj.Spec.Format == registryv1.FormatServerPackage && strings.TrimSpace(repoObj.Spec.DisplayName) != "" {
		title = strings.TrimSpace(repoObj.Spec.DisplayName)
	}
	p := &registryv1.Package{
		Repository:    repoObj.Metadata.Name,
		Package:       art.Spec.Package,
		Title:         title,
		LatestVersion: art.Spec.Version,
		ModIDs:        append([]string(nil), art.Status.ModIDs...),
		MapFolders:    append([]string(nil), art.Status.MapFolders...),
		ConfigSchema:  append([]registryv1.AddonConfigField(nil), art.Status.ConfigSchema...),
	}
	if art.Status.Descriptor != nil && art.Status.Descriptor.SizeBytes > 0 {
		p.SizeBytes = art.Status.Descriptor.SizeBytes
	}
	p.DefaultMeta()
	return p
}

func (o *operator) signRepoPackageIcons(ctx context.Context, repoObj *registryv1.Repository, items []*registryv1.Package) {
	if len(items) == 0 || repoObj == nil {
		return
	}
	svc, src, ok := o.proxyMirrorForRepo(ctx, repoObj)
	if !ok {
		return
	}
	for i, item := range items {
		if item != nil && item.Repository == repoObj.Metadata.Name {
			items[i] = signPackageIconWithService(svc, src.ImageProxyPrefix, item)
		}
	}
}

func (o *operator) normalizeClusterPackageList(
	ctx context.Context,
	list *metav1.List[*registryv1.Package],
	cachedOnly, refreshUpstream bool,
) {
	if o.repoDB == nil || list == nil {
		return
	}
	repoList, err := o.repoDB.List(ctx, db.ListOptions{SkipTotalCount: true})
	if err != nil || repoList == nil {
		return
	}
	if cachedOnly && !refreshUpstream {
		for _, repoObj := range repoList.Items {
			o.normalizeRepoPackageList(ctx, repoObj, list, true, false)
		}
		return
	}
	o.refreshClusterPackagesConcurrently(ctx, repoList.Items, list, cachedOnly, refreshUpstream)
}

func (o *operator) refreshClusterPackagesConcurrently(
	ctx context.Context,
	repos []*registryv1.Repository,
	list *metav1.List[*registryv1.Package],
	cachedOnly, refreshUpstream bool,
) {
	type repoSliceResult struct {
		indices []int
		items   []*registryv1.Package
	}
	results := make([]repoSliceResult, len(repos))
	var wg sync.WaitGroup
	for rIdx, repoObj := range repos {
		if repoObj == nil || repoObj.Spec.Mode != registryv1.RepositoryModeProxy {
			continue
		}
		indices, subset := extractRepoPackageSubset(repoObj.Metadata.Name, list.Items)
		results[rIdx].indices = indices
		wg.Add(1)
		go func(slot int, r *registryv1.Repository, sub []*registryv1.Package) {
			defer wg.Done()
			rCtx, cancel := context.WithTimeout(ctx, upstreamRefreshTimeout)
			defer cancel()
			subList := &metav1.List[*registryv1.Package]{Items: slices.Clone(sub)}
			o.normalizeRepoPackageList(rCtx, r, subList, cachedOnly, refreshUpstream)
			results[slot].items = subList.Items
		}(rIdx, repoObj, subset)
	}
	wg.Wait()
	for _, res := range results {
		for j, idx := range res.indices {
			list.Items[idx] = res.items[j]
		}
		if len(res.items) > len(res.indices) {
			added := res.items[len(res.indices):]
			list.Items = append(list.Items, added...)
			list.Metadata.TotalCount += len(added)
		}
	}
}

func extractRepoPackageSubset(repoName string, items []*registryv1.Package) ([]int, []*registryv1.Package) {
	var indices []int
	var subset []*registryv1.Package
	for i, item := range items {
		if item != nil && item.Repository == repoName {
			indices = append(indices, i)
			subset = append(subset, item)
		}
	}
	return indices, subset
}

func (o *operator) enrichServerItemsForRepo(
	ctx context.Context,
	repoObj *registryv1.Repository,
	items []*registryv1.Package,
) {
	for i, item := range items {
		if item != nil && item.Repository == repoObj.Metadata.Name {
			items[i] = o.enrichServerProxyPackage(ctx, repoObj, item)
		}
	}
}
