package v1

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func validateDNSName(field, val string) error {
	trimmed := strings.TrimSpace(val)
	if !dnsLabelRegex.MatchString(trimmed) {
		return fmt.Errorf("%s %q must be a lowercase DNS-1123 label", field, val)
	}
	return nil
}

func validateOptionalHTTPURL(field, raw string, allowSocks bool) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%s %q must be a valid URL", field, raw)
	}
	switch u.Scheme {
	case "http", "https":
		return nil
	case "socks5", "socks5h":
		if allowSocks {
			return nil
		}
	}
	return fmt.Errorf("%s %q has unsupported URL scheme %q", field, raw, u.Scheme)
}

func validateArtifactSourceURL(field, raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, PackageSourceSteamCMDScheme) {
		appIDStr := strings.TrimPrefix(trimmed, PackageSourceSteamCMDScheme)
		appID, err := strconv.Atoi(appIDStr)
		if err != nil || appID <= 0 {
			return fmt.Errorf("%s %q must specify a positive steamAppId", field, raw)
		}
		return nil
	}
	return validateOptionalHTTPURL(field, trimmed, false)
}

func (u *Upstream) DefaultMeta() {
	u.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: KindUpstream}
	u.Metadata.Name = strings.ToLower(strings.TrimSpace(u.Metadata.Name))
	u.Provider = strings.ToLower(strings.TrimSpace(u.Provider))
}

func (u *Upstream) Validate() error {
	if err := validateDNSName("upstream metadata.name", u.Metadata.Name); err != nil {
		return err
	}
	switch u.Provider {
	case UpstreamProviderPaperMC, UpstreamProviderPurpur, UpstreamProviderVanilla,
		UpstreamProviderFabricMC, UpstreamProviderNeoForge, UpstreamProviderModrinth, UpstreamProviderSteam, UpstreamProviderHTTP:
	default:
		return fmt.Errorf("unsupported upstream provider %q", u.Provider)
	}
	if u.Provider == UpstreamProviderSteam && u.SteamAppID <= 0 {
		return errors.New("upstream steamAppId must be positive for steam provider")
	}
	if err := validateOptionalHTTPURL("upstream.endpoint", u.Endpoint, false); err != nil {
		return err
	}
	return validateOptionalHTTPURL("upstream.proxyURL", u.ProxyURL, true)
}

func (r *Repository) DefaultMeta() {
	r.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: KindRepository}
	r.Metadata.Name = strings.ToLower(strings.TrimSpace(r.Metadata.Name))
	if strings.TrimSpace(r.Spec.Mode) == "" {
		r.Spec.Mode = RepositoryModeHosted
	}
	if strings.TrimSpace(r.Spec.Format) == "" {
		r.Spec.Format = FormatGeneric
	}
}

func (r *Repository) Validate() error {
	if err := validateDNSName("repository metadata.name", r.Metadata.Name); err != nil {
		return err
	}
	switch r.Spec.Mode {
	case RepositoryModeHosted:
	case RepositoryModeProxy:
		if strings.TrimSpace(r.Spec.UpstreamName) == "" {
			return errors.New("proxy repository requires spec.upstreamName")
		}
	case RepositoryModeVirtual:
		if len(r.Spec.MemberRepos) == 0 {
			return errors.New("virtual repository requires at least one spec.memberRepos entry")
		}
	default:
		return fmt.Errorf("unsupported repository spec.mode %q", r.Spec.Mode)
	}
	switch r.Spec.Format {
	case FormatServerPackage, FormatWorkshopMod, FormatModrinthAddon, FormatGeneric:
		return nil
	default:
		return fmt.Errorf("unsupported repository spec.format %q", r.Spec.Format)
	}
}

func (p *Package) DefaultMeta() {
	p.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: KindPackage}
	p.Repository = strings.ToLower(strings.TrimSpace(p.Repository))
	p.Package = strings.TrimSpace(p.Package)
	if p.Metadata.Name == "" && p.Repository != "" && p.Package != "" {
		p.Metadata.Name = FormatPackageName(p.Repository, p.Package)
	}
	if p.Title == "" {
		p.Title = p.Package
	}
}

func (p *Package) Validate() error {
	if err := validateDNSName("package repository", p.Repository); err != nil {
		return err
	}
	if p.Package == "" {
		return errors.New("package identifier is required")
	}
	expected := FormatPackageName(p.Repository, p.Package)
	if expected == "" || p.Metadata.Name != expected {
		return fmt.Errorf("package metadata.name %q must match %q", p.Metadata.Name, expected)
	}
	return nil
}

func (a *Artifact) DefaultMeta() {
	a.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: KindArtifact}
	a.Spec.Repository = strings.ToLower(strings.TrimSpace(a.Spec.Repository))
	a.Spec.Package = strings.TrimSpace(a.Spec.Package)
	a.Spec.Version = strings.TrimSpace(a.Spec.Version)
	if a.Metadata.Name == "" && a.Spec.Repository != "" && a.Spec.Package != "" && a.Spec.Version != "" {
		a.Metadata.Name = FormatArtifactName(a.Spec.Repository, a.Spec.Package, a.Spec.Version)
	}
	if a.Spec.ArchiveFormat == "" {
		a.Spec.ArchiveFormat = ArchiveFormatRaw
	}
}

func (a *Artifact) Validate() error {
	if err := validateDNSName("artifact spec.repository", a.Spec.Repository); err != nil {
		return err
	}
	if a.Spec.Package == "" || a.Spec.Version == "" {
		return errors.New("artifact spec.package and spec.version are required")
	}
	expected := FormatArtifactName(a.Spec.Repository, a.Spec.Package, a.Spec.Version)
	if expected == "" || a.Metadata.Name != expected {
		return fmt.Errorf("artifact metadata.name %q must match %q", a.Metadata.Name, expected)
	}
	if a.Spec.Descriptor != nil {
		canon, _, err := NormalizeDigest(a.Spec.Descriptor.Digest)
		if err != nil {
			return err
		}
		a.Spec.Descriptor.Digest = canon
	}
	return validateArtifactSourceURL("artifact spec.sourceURL", a.Spec.SourceURL)
}

func (b *Blob) DefaultMeta() {
	b.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: KindBlob}
	if canon, _, err := NormalizeDigest(b.Digest); err == nil {
		b.Digest = canon
	} else if canonMeta, _, errMeta := NormalizeDigest(b.Metadata.Name); errMeta == nil {
		b.Digest = canonMeta
	}
	if b.Digest != "" {
		b.Metadata.Name = b.Digest
		if b.ObjectKey == "" {
			b.ObjectKey, _ = BlobObjectKey(b.Digest)
		}
	}
	if b.MediaType == "" {
		b.MediaType = "application/octet-stream"
	}
}

func (b *Blob) Validate() error {
	canon, _, err := NormalizeDigest(b.Digest)
	if err != nil {
		return err
	}
	if b.Metadata.Name != canon {
		return fmt.Errorf("blob metadata.name %q must equal digest %q", b.Metadata.Name, canon)
	}
	if b.SizeBytes < 0 || b.RefCount < 0 {
		return errors.New("blob sizeBytes and refCount must be non-negative")
	}
	if strings.TrimSpace(b.ObjectKey) == "" {
		return errors.New("blob objectKey is required")
	}
	return nil
}

func (u *BlobUpload) DefaultMeta() {
	u.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: KindBlobUpload}
	u.Metadata.Name = strings.TrimSpace(u.Metadata.Name)
	if u.MediaType == "" {
		u.MediaType = "application/octet-stream"
	}
	if u.UploadState == "" {
		u.UploadState = BlobUploadStateUploading
	}
}

func (u *BlobUpload) Validate() error {
	if u.Metadata.Name == "" {
		return errors.New("blobUpload metadata.name is required")
	}
	if u.SizeBytes < 0 {
		return errors.New("blobUpload sizeBytes must be non-negative")
	}
	if u.Digest != "" {
		canon, _, err := NormalizeDigest(u.Digest)
		if err != nil {
			return err
		}
		u.Digest = canon
	}
	return nil
}

func (p *ReplicationPolicy) DefaultMeta() {
	p.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: KindReplicationPolicy}
	p.Metadata.Name = strings.ToLower(strings.TrimSpace(p.Metadata.Name))
	p.Repository = strings.ToLower(strings.TrimSpace(p.Repository))
	if p.Mode == "" {
		p.Mode = ReplicationModeTrackReleases
	}
	if p.IntervalMinutes <= 0 {
		p.IntervalMinutes = 60
	}
}

func (p *ReplicationPolicy) Validate() error {
	if err := validateDNSName("replicationPolicy metadata.name", p.Metadata.Name); err != nil {
		return err
	}
	if err := validateDNSName("replicationPolicy repository", p.Repository); err != nil {
		return err
	}
	switch p.Mode {
	case ReplicationModeTrackReleases:
		return nil
	case ReplicationModeTopPopular:
		if p.TopN <= 0 || p.TopN > 500 {
			return errors.New("replicationPolicy topN must be between 1 and 500 for TopPopular mode")
		}
		return nil
	case ReplicationModeExplicitPackages:
		if len(p.Packages) == 0 {
			return errors.New("replicationPolicy packages is required for ExplicitPackages mode")
		}
		return nil
	default:
		return fmt.Errorf("unsupported replicationPolicy mode %q", p.Mode)
	}
}

func (l *ArtifactLogChunk) DefaultMeta() {
	l.TypeMeta = metav1.TypeMeta{APIVersion: GroupVersion, Kind: KindArtifactLogChunk}
	if l.Action == "" {
		l.Action = ArtifactLogActionAppend
	}
}

func (l *ArtifactLogChunk) Validate() error {
	switch l.Action {
	case ArtifactLogActionReset, ArtifactLogActionAppend, ArtifactLogActionComplete:
		return nil
	default:
		return fmt.Errorf("unsupported artifact log action %q", l.Action)
	}
}
