package assets

import (
	"strings"
	"testing"
)

func TestPublishedVersionValidation(t *testing.T) {
	valid := PublishedVersion{AssetID: "world", OrganizationID: "tenant", Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 0}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*PublishedVersion){
		"empty owner":         func(v *PublishedVersion) { v.OrganizationID = "" },
		"invalid identity":    func(v *PublishedVersion) { v.AssetID = "x\x00" },
		"missing version":     func(v *PublishedVersion) { v.Version = "" },
		"negative size":       func(v *PublishedVersion) { v.SizeBytes = -1 },
		"short digest":        func(v *PublishedVersion) { v.SHA256 = "aa" },
		"nonhex digest":       func(v *PublishedVersion) { v.SHA256 = strings.Repeat("x", 64) },
		"noncanonical digest": func(v *PublishedVersion) { v.SHA256 = strings.Repeat("A", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			v := valid
			change(&v)
			if v.Validate() != ErrInvalidVersion {
				t.Fatal("invalid version accepted")
			}
		})
	}
}
