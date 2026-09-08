package instances

import (
	"errors"
	"math"
	"testing"
)

func TestRejectIncompleteGlobalIntent(t *testing.T) {
	valid := CreateRequest{OrganizationID: "org", Name: "server", RegionID: "east", IdempotencyKey: "create", Specification: Specification{ProviderKey: "plugin", GameVersion: "1.0", ConfigSchemaVersion: 1, Configuration: ProtectedConfiguration{KeyID: "key-v1", Ciphertext: []byte("protected")}, Resources: Resources{CPU: 1, MemoryMB: 512}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*CreateRequest){
		"region missing":     func(r *CreateRequest) { r.RegionID = "" },
		"region whitespace":  func(r *CreateRequest) { r.RegionID = " east " },
		"key missing":        func(r *CreateRequest) { r.IdempotencyKey = "" },
		"unbounded CPU":      func(r *CreateRequest) { r.Specification.Resources.CPU = 0 },
		"NaN CPU":            func(r *CreateRequest) { r.Specification.Resources.CPU = math.NaN() },
		"infinite CPU":       func(r *CreateRequest) { r.Specification.Resources.CPU = math.Inf(1) },
		"negative memory":    func(r *CreateRequest) { r.Specification.Resources.MemoryMB = -1 },
		"unprotected config": func(r *CreateRequest) { r.Specification.Configuration = ProtectedConfiguration{} },
		"unversioned asset":  func(r *CreateRequest) { r.Specification.Assets = []AssetVersion{{AssetID: "world"}} },
		"conflicting assets": func(r *CreateRequest) { r.Specification.Assets = []AssetVersion{{"world", "v1"}, {"world", "v2"}} },
	} {
		t.Run(name, func(t *testing.T) {
			request := valid
			mutate(&request)
			if err := request.Validate(); !errors.Is(err, ErrInvalidIntent) {
				t.Fatalf("invalid intent accepted: %v", err)
			}
		})
	}
	if err := (ReviseRequest{OrganizationID: "org", ServerID: "server", ExpectedGeneration: math.MaxInt64, IdempotencyKey: "edit", Specification: valid.Specification}).Validate(); !errors.Is(err, ErrInvalidIntent) {
		t.Fatalf("generation overflow accepted: %v", err)
	}
}
