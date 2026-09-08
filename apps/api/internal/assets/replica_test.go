package assets

import "testing"

func TestReplicaRegistration(t *testing.T) {
	r := Replica{ID: "replica", AssetID: "asset", AssetVersion: "v1", RegionID: "east", StorageID: "store"}
	if err := r.ValidateRegistration(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Replica){func(r *Replica) { r.Available = true }, func(r *Replica) { r.Version = 1 }, func(r *Replica) { r.StorageID = "https://host" }, func(r *Replica) { r.RegionID = "../east" }, func(r *Replica) { r.ID = "" }, func(r *Replica) { r.AssetVersion = "" }} {
		bad := r
		mutate(&bad)
		if bad.ValidateRegistration() == nil {
			t.Fatal("invalid replica registration accepted")
		}
	}
}
