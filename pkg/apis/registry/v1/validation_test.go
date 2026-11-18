package v1

import (
	"strings"
	"testing"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestCoordinatesAndDigests(t *testing.T) {
	hex64 := strings.Repeat("a", 64)
	canon, hexPart, err := NormalizeDigest("SHA256:" + strings.ToUpper(hex64))
	if err != nil {
		t.Fatalf("NormalizeDigest failed: %v", err)
	}
	if canon != "sha256:"+hex64 || hexPart != hex64 {
		t.Fatalf("unexpected normalized digest: %q / %q", canon, hexPart)
	}
	key, err := BlobObjectKey(canon)
	if err != nil || key != "blobs/sha256/aa/"+hex64 {
		t.Fatalf("unexpected BlobObjectKey: %q, err=%v", key, err)
	}
	if _, _, err := NormalizeDigest("md5:1234"); err == nil {
		t.Fatal("expected NormalizeDigest error on invalid algorithm")
	}

	pkgCoord := FormatPackageName("mc-paper", "coreprotect")
	repo, pkg, err := ParsePackageName(pkgCoord)
	if err != nil || repo != "mc-paper" || pkg != "coreprotect" {
		t.Fatalf("unexpected ParsePackageName(%q): %q, %q, %v", pkgCoord, repo, pkg, err)
	}

	artCoord := FormatArtifactName("mc-paper", "paper.server", "1.21.4-build.222")
	repo, pkg, ver, err := ParseArtifactName(artCoord)
	if err != nil || repo != "mc-paper" || pkg != "paper-server" || ver != "1.21.4-build.222" {
		t.Fatalf("unexpected ParseArtifactName(%q): %q, %q, %q, %v", artCoord, repo, pkg, ver, err)
	}
}

func TestValidateUpstreamAndRepository(t *testing.T) {
	up := &Upstream{
		Metadata:   metav1.ObjectMeta{Name: "steam-workshop-dst"},
		Provider:   UpstreamProviderSteam,
		SteamAppID: 322330,
	}
	up.DefaultMeta()
	if err := up.Validate(); err != nil {
		t.Fatalf("valid Upstream failed Validate: %v", err)
	}
	badUp := &Upstream{Metadata: metav1.ObjectMeta{Name: "bad"}, Provider: UpstreamProviderSteam}
	if err := badUp.Validate(); err == nil {
		t.Fatal("expected error for Steam Upstream without steamAppId")
	}

	repo := &Repository{
		Metadata: metav1.ObjectMeta{Name: "dst-mods"},
		Spec: RepositorySpec{
			Mode:         RepositoryModeProxy,
			Format:       FormatWorkshopMod,
			UpstreamName: "steam-workshop-dst",
		},
	}
	repo.DefaultMeta()
	if err := repo.Validate(); err != nil {
		t.Fatalf("valid Repository failed Validate: %v", err)
	}
	badRepo := &Repository{
		Metadata: metav1.ObjectMeta{Name: "bad.dot"},
		Spec:     RepositorySpec{Mode: RepositoryModeHosted, Format: FormatGeneric},
	}
	if err := badRepo.Validate(); err == nil {
		t.Fatal("expected error for Repository name containing dot")
	}
}

func TestValidateArtifactsBlobsAndPolicies(t *testing.T) {
	validDigest := "sha256:" + strings.Repeat("b", 64)

	pkg := &Package{Repository: "dst-mods", Package: "378160973", Title: "Global Positions"}
	pkg.DefaultMeta()
	if pkg.Metadata.Name != "dst-mods.378160973" || pkg.Validate() != nil {
		t.Fatalf("unexpected Package validation state: %+v", pkg)
	}

	art := &Artifact{
		Spec: ArtifactSpec{
			Repository:    "dst-mods",
			Package:       "378160973",
			Version:       "1715000123",
			ArchiveFormat: ArchiveFormatZip,
			Descriptor:    &BlobDescriptor{Digest: validDigest, SizeBytes: 1024},
		},
	}
	art.DefaultMeta()
	if art.Metadata.Name != "dst-mods.378160973.1715000123" || art.Validate() != nil {
		t.Fatalf("unexpected Artifact validation state: %+v", art)
	}

	blob := &Blob{Digest: validDigest, SizeBytes: 1024}
	blob.DefaultMeta()
	if err := blob.Validate(); err != nil {
		t.Fatalf("valid Blob failed Validate: %v", err)
	}

	bu := &BlobUpload{Metadata: metav1.ObjectMeta{Name: "up-01"}, Digest: validDigest, SizeBytes: 1024}
	bu.DefaultMeta()
	if err := bu.Validate(); err != nil {
		t.Fatalf("valid BlobUpload failed Validate: %v", err)
	}

	pol := &ReplicationPolicy{
		Metadata:   metav1.ObjectMeta{Name: "sync-dst-top"},
		Repository: "dst-mods",
		Mode:       ReplicationModeExplicitPackages,
		Packages:   []string{"378160973"},
		Enabled:    true,
	}
	pol.DefaultMeta()
	if err := pol.Validate(); err != nil {
		t.Fatalf("valid ReplicationPolicy failed Validate: %v", err)
	}
}

func TestValidateArtifactSourceURL(t *testing.T) {
	baseSpec := func(src string) *Artifact {
		a := &Artifact{
			Spec: ArtifactSpec{
				Repository: "dst-server",
				Package:    "dst-server",
				Version:    "latest",
				SourceURL:  src,
			},
		}
		a.DefaultMeta()
		return a
	}
	for _, valid := range []string{"", "steamcmd://343050", "https://example.com/server.tar.gz"} {
		if err := baseSpec(valid).Validate(); err != nil {
			t.Fatalf("expected sourceURL %q to validate cleanly, got %v", valid, err)
		}
	}
	for _, invalid := range []string{"steamcmd://0", "steamcmd://abc", "ftp://example.com/server.tar.gz"} {
		if err := baseSpec(invalid).Validate(); err == nil {
			t.Fatalf("expected sourceURL %q to fail validation", invalid)
		}
	}
}
