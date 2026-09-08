// Package transferauth issues and verifies bounded asset-transfer capabilities.
// Signing stays global; source stores receive only verification public keys.
package transferauth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

const purpose = "gamepanel.asset-transfer.v1"

var ErrInvalidTicket = errors.New("invalid asset transfer ticket")

type SourceResolver interface {
	ResolveRegionalAssetSource(context.Context, string, instances.RevisionAvailable, instances.AssetVersion, string, int64) (regional.AssetSourceSnapshot, error)
}

type Grant struct {
	Version     int                          `json:"version"`
	Purpose     string                       `json:"purpose"`
	KeyID       string                       `json:"keyId"`
	ID          string                       `json:"id"`
	IssuedAtMS  int64                        `json:"issuedAtMs"`
	ExpiresAtMS int64                        `json:"expiresAtMs"`
	Source      regional.AssetSourceSnapshot `json:"source"`
}

type IssuerOptions struct {
	KeyID         string
	PrivateKey    ed25519.PrivateKey
	Resolver      SourceResolver
	TTL           time.Duration
	MaxTokenBytes int
	Now           func() time.Time
}
type Issuer struct{ options IssuerOptions }

func NewIssuer(o IssuerOptions) (*Issuer, error) {
	if !validKeyID(o.KeyID) || len(o.PrivateKey) != ed25519.PrivateKeySize || o.Resolver == nil || !validTTL(o.TTL) || o.MaxTokenBytes < 1 || o.Now == nil {
		return nil, ErrInvalidTicket
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(o.PrivateKey[:ed25519.SeedSize]), o.PrivateKey) {
		return nil, ErrInvalidTicket
	}
	o.PrivateKey = append(ed25519.PrivateKey(nil), o.PrivateKey...)
	return &Issuer{options: o}, nil
}

// Issue always resolves current authorization; a caller cannot submit its own
// source snapshot. The expiry starts before that read, not after a slow lookup.
func (i *Issuer) Issue(ctx context.Context, authenticatedTarget string, event instances.RevisionAvailable, ref instances.AssetVersion, replicaID string, replicaVersion int64) (string, error) {
	if event.Validate() != nil || authenticatedTarget != event.RegionID {
		return "", ErrInvalidTicket
	}
	o := i.options
	start := o.Now()
	deadline := start.Add(o.TTL)
	workCtx, cancel := context.WithTimeout(ctx, o.TTL)
	defer cancel()
	source, err := o.Resolver.ResolveRegionalAssetSource(workCtx, authenticatedTarget, event, ref, replicaID, replicaVersion)
	if err != nil {
		return "", err
	}
	if source.ValidateFor(event, ref, replicaID, replicaVersion) != nil {
		return "", ErrInvalidTicket
	}
	if err := workCtx.Err(); err != nil {
		return "", err
	}
	now := o.Now()
	if now.Before(start) || !now.Before(deadline) || start.UnixMilli() <= 0 || deadline.UnixMilli() <= start.UnixMilli() {
		return "", ErrInvalidTicket
	}
	grant := Grant{Version: 1, Purpose: purpose, KeyID: o.KeyID, ID: rand.Text(), IssuedAtMS: start.UnixMilli(), ExpiresAtMS: deadline.UnixMilli(), Source: source}
	payload, err := json.Marshal(grant)
	if err != nil {
		return "", err
	}
	signature := ed25519.Sign(o.PrivateKey, payload)
	token := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature)
	if len(token) > o.MaxTokenBytes {
		return "", ErrInvalidTicket
	}
	if err := workCtx.Err(); err != nil {
		return "", err
	}
	return token, nil
}

type VerifierOptions struct {
	SourceRegion, StorageID string
	PublicKeys              map[string]ed25519.PublicKey
	MaxTTL                  time.Duration
	MaxTokenBytes           int
	Now                     func() time.Time
}
type Verifier struct{ options VerifierOptions }

func NewVerifier(o VerifierOptions) (*Verifier, error) {
	if o.SourceRegion == "" || o.StorageID == "" || len(o.PublicKeys) == 0 || !validTTL(o.MaxTTL) || o.MaxTokenBytes < 1 || o.Now == nil {
		return nil, ErrInvalidTicket
	}
	keys := make(map[string]ed25519.PublicKey, len(o.PublicKeys))
	for id, key := range o.PublicKeys {
		if !validKeyID(id) || len(key) != ed25519.PublicKeySize {
			return nil, ErrInvalidTicket
		}
		keys[id] = append(ed25519.PublicKey(nil), key...)
	}
	o.PublicKeys = keys
	return &Verifier{options: o}, nil
}

// Verify requires the authenticated requesting Region, not a ticket/header
// claim. Callers must enforce ExpiresAtMS for the entire file transfer.
func (v *Verifier) Verify(ctx context.Context, token, authenticatedTarget string) (Grant, error) {
	if err := ctx.Err(); err != nil {
		return Grant{}, err
	}
	o := v.options
	if len(token) > o.MaxTokenBytes || authenticatedTarget == "" {
		return Grant{}, ErrInvalidTicket
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Grant{}, ErrInvalidTicket
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return Grant{}, ErrInvalidTicket
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Grant{}, ErrInvalidTicket
	}
	var grant Grant
	if json.Unmarshal(payload, &grant) != nil {
		return Grant{}, ErrInvalidTicket
	}
	key, ok := o.PublicKeys[grant.KeyID]
	if !ok || !ed25519.Verify(key, payload, signature) {
		return Grant{}, ErrInvalidTicket
	}
	// Only the issuer's canonical encoding is accepted: no duplicate fields,
	// aliases, unknown extensions or alternative parses of signed claims.
	canonical, err := json.Marshal(grant)
	if err != nil || !bytes.Equal(canonical, payload) {
		return Grant{}, ErrInvalidTicket
	}
	now := o.Now().UnixMilli()
	if grant.Version != 1 || grant.Purpose != purpose || grant.ID == "" || grant.IssuedAtMS <= 0 || grant.ExpiresAtMS <= grant.IssuedAtMS || grant.ExpiresAtMS-grant.IssuedAtMS > o.MaxTTL.Milliseconds() || now < grant.IssuedAtMS || now >= grant.ExpiresAtMS {
		return Grant{}, ErrInvalidTicket
	}
	s := grant.Source
	if s.Event.RegionID != authenticatedTarget || s.Replica.RegionID != o.SourceRegion || s.Replica.StorageID != o.StorageID || s.ValidateFor(s.Event, instances.AssetVersion{AssetID: s.Asset.AssetID, Version: s.Asset.Version}, s.Replica.ID, s.Replica.Version) != nil {
		return Grant{}, ErrInvalidTicket
	}
	if err := ctx.Err(); err != nil {
		return Grant{}, err
	}
	return grant, nil
}

func validKeyID(id string) bool {
	return id != "" && len(id) <= 128 && strings.TrimSpace(id) == id && !strings.ContainsAny(id, "\x00\r\n")
}
func validTTL(ttl time.Duration) bool {
	return ttl >= time.Millisecond && ttl <= time.Hour && ttl%time.Millisecond == 0
}
