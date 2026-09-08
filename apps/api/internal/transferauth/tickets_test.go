package transferauth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type resolverFunc func(context.Context, string, instances.RevisionAvailable, instances.AssetVersion, string, int64) (regional.AssetSourceSnapshot, error)

func (f resolverFunc) ResolveRegionalAssetSource(ctx context.Context, r string, e instances.RevisionAvailable, a instances.AssetVersion, id string, v int64) (regional.AssetSourceSnapshot, error) {
	return f(ctx, r, e, a, id, v)
}

func TestTransferTickets(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	e := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "org", ServerID: "server", RevisionID: "revision", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	ref := instances.AssetVersion{AssetID: "world", Version: "v1"}
	source := regional.AssetSourceSnapshot{Event: e, Asset: assets.PublishedVersion{OrganizationID: "org", AssetID: "world", Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 1}, Replica: assets.Replica{ID: "replica", AssetID: "world", AssetVersion: "v1", RegionID: "west", StorageID: "store", Available: true, Version: 2}}
	calls := 0
	deny := false
	slow := false
	resolver := resolverFunc(func(_ context.Context, r string, event instances.RevisionAvailable, a instances.AssetVersion, id string, version int64) (regional.AssetSourceSnapshot, error) {
		calls++
		if r != "east" || event != e || a != ref || id != "replica" || version != 2 {
			t.Fatal("resolver lost request identity")
		}
		if deny {
			return regional.AssetSourceSnapshot{}, regional.ErrRevisionUnavailable
		}
		if slow {
			now = now.Add(time.Minute)
		}
		return source, nil
	})
	issuerKey := append(ed25519.PrivateKey(nil), private...)
	issuer, err := NewIssuer(IssuerOptions{KeyID: "key", PrivateKey: issuerKey, Resolver: resolver, TTL: time.Minute, MaxTokenBytes: 16384, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	clear(issuerKey)
	keys := map[string]ed25519.PublicKey{"key": append(ed25519.PublicKey(nil), public...)}
	opts := VerifierOptions{SourceRegion: "west", StorageID: "store", PublicKeys: keys, MaxTTL: time.Minute, MaxTokenBytes: 16384, Now: func() time.Time { return now }}
	verifier, err := NewVerifier(opts)
	if err != nil {
		t.Fatal(err)
	}
	keys["key"][0] ^= 1
	delete(keys, "key")
	token, err := issuer.Issue(ctx, "east", e, ref, "replica", 2)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := verifier.Verify(ctx, token, "east")
	if err != nil || grant.Source != source || grant.ExpiresAtMS-grant.IssuedAtMS != 60000 {
		t.Fatalf("ticket roundtrip: %v", err)
	}
	second, err := issuer.Issue(ctx, "east", e, ref, "replica", 2)
	if err != nil || second == token || calls != 2 {
		t.Fatal("issuance did not reauthorize or reuse was ambiguous")
	}
	deny = true
	if got, err := issuer.Issue(ctx, "east", e, ref, "replica", 2); !errors.Is(err, regional.ErrRevisionUnavailable) || got != "" {
		t.Fatal("revoked access still issued")
	}
	deny = false
	if _, err := verifier.Verify(ctx, token, "other"); err == nil {
		t.Fatal("ticket accepted from other target")
	}
	for _, change := range []func(*VerifierOptions){func(o *VerifierOptions) { o.SourceRegion = "other" }, func(o *VerifierOptions) { o.StorageID = "other" }, func(o *VerifierOptions) { o.MaxTTL = time.Second }} {
		o := opts
		o.PublicKeys = map[string]ed25519.PublicKey{"key": public}
		change(&o)
		v, err := NewVerifier(o)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := v.Verify(ctx, token, "east"); err == nil {
			t.Fatal("ticket accepted by wrong source or lifetime policy")
		}
	}
	parts := strings.Split(token, ".")
	payload, _ := base64.RawURLEncoding.DecodeString(parts[0])
	for i := range payload {
		changed := append([]byte(nil), payload...)
		changed[i] ^= 1
		if _, err := verifier.Verify(ctx, base64.RawURLEncoding.EncodeToString(changed)+"."+parts[1], "east"); err == nil {
			t.Fatalf("payload mutation %d accepted", i)
		}
	}
	signature, _ := base64.RawURLEncoding.DecodeString(parts[1])
	signature[0] ^= 1
	if _, err := verifier.Verify(ctx, parts[0]+"."+base64.RawURLEncoding.EncodeToString(signature), "east"); err == nil {
		t.Fatal("signature mutation accepted")
	}
	signed := func(g Grant) string {
		raw, _ := json.Marshal(g)
		return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, raw))
	}
	duplicate := []byte(strings.TrimSuffix(string(payload), "}") + `,"version":1}`)
	duplicateToken := base64.RawURLEncoding.EncodeToString(duplicate) + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(private, duplicate))
	if _, err := verifier.Verify(ctx, duplicateToken, "east"); err == nil {
		t.Fatal("duplicate signed fields accepted")
	}
	for _, change := range []func(*Grant){func(g *Grant) { g.Purpose = "execution" }, func(g *Grant) { g.Version = 2 }, func(g *Grant) { g.ExpiresAtMS = g.IssuedAtMS }, func(g *Grant) { g.ExpiresAtMS += 60000 }, func(g *Grant) { g.Source.Asset.OrganizationID = "foreign" }, func(g *Grant) { g.Source.Replica.Available = false }, func(g *Grant) { g.KeyID = "missing" }} {
		bad := grant
		change(&bad)
		if _, err := verifier.Verify(ctx, signed(bad), "east"); err == nil {
			t.Fatal("invalid signed claims accepted")
		}
	}
	now = now.Add(-time.Millisecond)
	if _, err := verifier.Verify(ctx, token, "east"); err == nil {
		t.Fatal("future ticket accepted")
	}
	now = time.UnixMilli(grant.ExpiresAtMS)
	if _, err := verifier.Verify(ctx, token, "east"); err == nil {
		t.Fatal("expired ticket accepted")
	}
	slow = true
	if got, err := issuer.Issue(ctx, "east", e, ref, "replica", 2); err == nil || got != "" {
		t.Fatal("slow resolution extended ticket lifetime")
	}
	tooLarge := strings.Repeat("x", 16385)
	if _, err := verifier.Verify(ctx, tooLarge, "east"); err == nil {
		t.Fatal("oversized token accepted")
	}
}
