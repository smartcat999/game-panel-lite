package serviceauth

import (
	"errors"
	"net/http"
	"net/url"
)

// GlobalControls authenticates explicitly configured global control-plane
// client certificates at a regional operations listener.
type GlobalControls struct{ identities map[string]struct{} }

func NewGlobalControls(identities []string) (*GlobalControls, error) {
	if len(identities) == 0 {
		return nil, errors.New("global control certificate identities are required")
	}
	allowed := make(map[string]struct{}, len(identities))
	for _, identity := range identities {
		uri, err := url.Parse(identity)
		if err != nil || uri.Scheme == "" || uri.Host == "" || uri.User != nil || uri.RawQuery != "" || uri.Fragment != "" || identity != uri.String() {
			return nil, errors.New("invalid global control certificate identity")
		}
		if _, exists := allowed[identity]; exists {
			return nil, errors.New("duplicate global control certificate identity")
		}
		allowed[identity] = struct{}{}
	}
	return &GlobalControls{identities: allowed}, nil
}

func (g *GlobalControls) Authenticate(request *http.Request) error {
	identity, err := verifiedIdentity(request)
	if err != nil {
		return err
	}
	if _, allowed := g.identities[identity]; !allowed {
		return ErrUnauthenticated
	}
	return nil
}
