// Package serviceauth binds verified client certificates to configured regions.
package serviceauth

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnauthenticated = errors.New("regional service identity not verified")

type Regions struct{ identities map[string]string }

// NewRegions snapshots an operator-supplied URI SAN -> region allowlist.
// Issuers manage certificate lifetime; revocation requires replacing this
// allowlist or the trusted roots, and closing affected service connections.
func NewRegions(identities map[string]string) (*Regions, error) {
	if len(identities) == 0 {
		return nil, errors.New("regional certificate identities are required")
	}
	copy := make(map[string]string, len(identities))
	for identity, region := range identities {
		uri, err := url.Parse(identity)
		if err != nil || uri.Scheme == "" || uri.Host == "" || uri.User != nil || uri.RawQuery != "" || uri.Fragment != "" || identity != uri.String() || region == "" || region != strings.TrimSpace(region) {
			return nil, errors.New("invalid regional certificate identity mapping")
		}
		copy[identity] = region
	}
	return &Regions{identities: copy}, nil
}

func (r *Regions) Authenticate(request *http.Request) (string, error) {
	if request.TLS == nil || len(request.TLS.VerifiedChains) == 0 || len(request.TLS.PeerCertificates) == 0 {
		return "", ErrUnauthenticated
	}
	leaf := request.TLS.PeerCertificates[0]
	if len(leaf.URIs) != 1 {
		return "", ErrUnauthenticated
	}
	now := time.Now()
	valid := false
	for _, chain := range request.TLS.VerifiedChains {
		if len(chain) == 0 || !leaf.Equal(chain[0]) {
			continue
		}
		current := true
		for _, certificate := range chain {
			if now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
				current = false
				break
			}
		}
		if current {
			valid = true
			break
		}
	}
	if !valid {
		return "", ErrUnauthenticated
	}
	region, ok := r.identities[leaf.URIs[0].String()]
	if !ok {
		return "", ErrUnauthenticated
	}
	return region, nil
}

func ServerTLS(certificate tls.Certificate, clientCAs *x509.CertPool) (*tls.Config, error) {
	if len(certificate.Certificate) == 0 || certificate.PrivateKey == nil || clientCAs == nil {
		return nil, errors.New("server certificate and client CA pool are required")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs.Clone()}, nil
}
