package deliverycontrol

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

func NewAuthorityGrant(payload DesiredPayload, key []byte, expiresAt time.Time) (AuthorityGrant, error) {
	grant := AuthorityGrant{Issuer: "control-plane", Action: "deployment.reconcile", RegionID: payload.RegionID, PlacementVersion: payload.PlacementVersion, ExpiresAt: expiresAt.UTC()}
	signature, err := authoritySignature(payload, grant, key)
	if err != nil {
		return AuthorityGrant{}, err
	}
	grant.Signature = signature
	return grant, nil
}

func VerifyAuthority(payload DesiredPayload, key []byte, now time.Time) bool {
	grant := payload.AuthorityGrant
	if grant.Issuer != "control-plane" || grant.Action != "deployment.reconcile" || grant.RegionID != payload.RegionID || grant.PlacementVersion != payload.PlacementVersion || !now.Before(grant.ExpiresAt) {
		return false
	}
	expected, err := authoritySignature(payload, grant, key)
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(expected), []byte(grant.Signature))
}

func authoritySignature(payload DesiredPayload, grant AuthorityGrant, key []byte) (string, error) {
	payload.AuthorityGrant = AuthorityGrant{}
	encoded, err := json.Marshal(struct {
		Payload   DesiredPayload `json:"payload"`
		ExpiresAt time.Time      `json:"expiresAt"`
	}{Payload: payload, ExpiresAt: grant.ExpiresAt.UTC()})
	if err != nil {
		return "", err
	}
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(encoded)
	return hex.EncodeToString(digest.Sum(nil)), nil
}
