package domain

// NodeRequirements describes runtime constraints owned by a game provider.
// An empty architecture list imposes no additional architecture restriction.
type NodeRequirements struct {
	Architectures []string
}
