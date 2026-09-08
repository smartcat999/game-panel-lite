package serviceauth

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Nodes is an operator allowlist for a single regional listener. The listener's
// database must be opened for RegionID; neither headers nor request bodies set it.
type Nodes struct {
	regionID   string
	identities map[string]string
}

func NewNodes(region string, identities map[string]string) (*Nodes, error) {
	if region == "" || region != strings.TrimSpace(region) || len(identities) == 0 {
		return nil, errors.New("node region and certificate identities required")
	}
	copied := make(map[string]string, len(identities))
	for identity, node := range identities {
		uri, err := url.Parse(identity)
		if err != nil || uri.Scheme == "" || uri.Host == "" || uri.User != nil || uri.RawQuery != "" || uri.Fragment != "" || identity != uri.String() || node == "" || len(node) > 128 || node != strings.TrimSpace(node) || strings.ContainsAny(node, "\x00\r\n") {
			return nil, errors.New("invalid node certificate identity mapping")
		}
		copied[identity] = node
	}
	return &Nodes{regionID: region, identities: copied}, nil
}
func (n *Nodes) Authenticate(request *http.Request) (string, error) {
	identity, err := verifiedIdentity(request)
	if err != nil {
		return "", err
	}
	node, ok := n.identities[identity]
	if !ok {
		return "", ErrUnauthenticated
	}
	return node, nil
}

func (n *Nodes) RegionID() string { return n.regionID }
