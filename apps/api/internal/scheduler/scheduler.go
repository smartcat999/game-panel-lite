package scheduler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/scheduling"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

var (
	ErrNoNodesAvailable     = errors.New("no compute nodes registered")
	ErrNoFeasibleNodes      = errors.New("no online compute nodes satisfy placement requirements")
	ErrNodeNotFound         = errors.New("specified compute node not found")
	ErrNodeOffline          = errors.New("specified compute node is offline or heartbeat is stale")
	ErrPortConflict         = errors.New("host port conflicts with an existing server on the node")
	ErrArchIncompatible     = errors.New("node architecture is incompatible with game requirements")
	ErrInsufficientCapacity = scheduling.ErrCapacityUnavailable
	ErrNodeUnschedulable    = errors.New("node is cordoned / unschedulable")
)

const HeartbeatTimeout = 45 * time.Second

type Store interface {
	ListComputeNodes(context.Context) ([]domain.ComputeNode, error)
	GetComputeNode(context.Context, string) (domain.ComputeNode, error)
	ListGameServers(context.Context) ([]domain.GameServer, error)
}

type RequirementsResolver interface {
	NodeRequirements(domain.ProviderKey) (domain.NodeRequirements, error)
}

type Scheduler struct {
	localArchitecture func() string
	requirements      RequirementsResolver
	store             Store
	now               func() time.Time
}

func New(store Store, requirements RequirementsResolver) *Scheduler {
	return &Scheduler{
		store:        store,
		requirements: requirements,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *Scheduler) WithClock(clock func() time.Time) *Scheduler {
	s.now = clock
	return s
}

// ValidateNode validates whether a user-selected node is feasible for the target game server.
func (s *Scheduler) ValidateNode(ctx context.Context, nodeID string, server domain.GameServer) error {
	if strings.TrimSpace(nodeID) == "" {
		return nil
	}
	node, err := s.store.GetComputeNode(ctx, nodeID)
	if err != nil {
		return ErrNodeNotFound
	}
	existingServers, err := s.store.ListGameServers(ctx)
	if err != nil {
		return fmt.Errorf("list existing servers: %w", err)
	}

	now := s.now()
	if err := filterSchedulable(node); err != nil {
		return err
	}
	if err := filterAlive(node, now); err != nil {
		return err
	}
	if err := s.filterProviderMatch(node, server); err != nil {
		return err
	}
	if err := filterPortAvailable(node, server, existingServers); err != nil {
		return err
	}
	if err := filterCapacity(node, server, existingServers); err != nil {
		return err
	}
	return nil
}

// Schedule evaluates all registered compute nodes, filters out infeasible nodes,
// scores the feasible candidates, and returns the highest-scoring node.
func (s *Scheduler) Schedule(ctx context.Context, server domain.GameServer) (domain.ComputeNode, error) {
	nodes, err := s.store.ListComputeNodes(ctx)
	if err != nil {
		return domain.ComputeNode{}, fmt.Errorf("list compute nodes: %w", err)
	}
	if len(nodes) == 0 {
		return domain.ComputeNode{}, ErrNoNodesAvailable
	}

	existingServers, err := s.store.ListGameServers(ctx)
	if err != nil {
		return domain.ComputeNode{}, fmt.Errorf("list game servers: %w", err)
	}

	now := s.now()
	var candidates []domain.ComputeNode
	for _, node := range nodes {
		if err := filterSchedulable(node); err != nil {
			continue
		}
		if err := filterAlive(node, now); err != nil {
			continue
		}
		if err := s.filterProviderMatch(node, server); err != nil {
			continue
		}
		if err := filterPortAvailable(node, server, existingServers); err != nil {
			continue
		}
		if err := filterCapacity(node, server, existingServers); err != nil {
			continue
		}
		candidates = append(candidates, node)
	}

	if len(candidates) == 0 {
		return domain.ComputeNode{}, ErrNoFeasibleNodes
	}

	bestNode := candidates[0]
	bestScore := scoreLeastAllocated(candidates[0], existingServers)

	for i := 1; i < len(candidates); i++ {
		score := scoreLeastAllocated(candidates[i], existingServers)
		if score > bestScore {
			bestScore = score
			bestNode = candidates[i]
		}
	}

	return bestNode, nil
}

func filterSchedulable(node domain.ComputeNode) error {
	if node.Unschedulable {
		return ErrNodeUnschedulable
	}
	return nil
}

func filterAlive(node domain.ComputeNode, now time.Time) error {
	if node.Status == "offline" {
		return ErrNodeOffline
	}
	if !node.IsLocal {
		if node.LastHeartbeat.IsZero() || now.Sub(node.LastHeartbeat) > HeartbeatTimeout {
			return ErrNodeOffline
		}
	}
	return nil
}

func (s *Scheduler) filterProviderMatch(node domain.ComputeNode, server domain.GameServer) error {
	if s.requirements == nil {
		return fmt.Errorf("provider requirements resolver is required")
	}
	requirements, err := s.requirements.NodeRequirements(server.ProviderKey)
	if err != nil {
		return err
	}
	if len(requirements.Architectures) > 0 {
		architecture := workload.NormalizeArchitecture(node.RuntimeArchitecture)
		if node.IsLocal {
			architecture = ""
			if s.localArchitecture != nil {
				architecture = workload.NormalizeArchitecture(s.localArchitecture())
			}
		}
		if architecture == "" || !slices.Contains(requirements.Architectures, architecture) {
			return fmt.Errorf("%w: provider %s requires %v; node reports %q", ErrArchIncompatible, server.ProviderKey, requirements.Architectures, architecture)
		}
	}

	if len(server.Spec.ModIDs) > 0 && len(node.WorkloadCapabilities) > 0 {
		if !slices.Contains(node.WorkloadCapabilities, "artifacts-v1") {
			return fmt.Errorf("%w: node missing artifacts-v1 capability required for mod installation", ErrArchIncompatible)
		}
	}
	return nil
}

func filterPortAvailable(node domain.ComputeNode, server domain.GameServer, existing []domain.GameServer) error {
	hostPort := server.Spec.Network.HostPort
	if hostPort == 0 {
		hostPort = server.Spec.Network.Port
	}
	if hostPort <= 0 {
		return nil
	}

	for _, other := range existing {
		if other.ID == server.ID || other.NodeID != node.ID {
			continue
		}
		otherPort := other.Spec.Network.HostPort
		if otherPort == 0 {
			otherPort = other.Spec.Network.Port
		}
		if otherPort == hostPort {
			return fmt.Errorf("%w: port %d is already bound by server %q on node %s", ErrPortConflict, hostPort, other.Name, node.Name)
		}
	}
	return nil
}

func filterCapacity(node domain.ComputeNode, server domain.GameServer, existing []domain.GameServer) error {
	reserved := make([]scheduling.Resources, 0, len(existing))
	for _, other := range existing {
		if other.ID == server.ID || other.NodeID != node.ID {
			continue
		}
		reserved = append(reserved, scheduling.Resources{CPU: other.Spec.Resources.CPULimitCores, MemoryMB: int64(other.Spec.Resources.MemoryLimitMB)})
	}
	_, err := scheduling.CheckCapacity(
		scheduling.Resources{CPU: float64(node.CPUCores), MemoryMB: node.MemoryTotalMB},
		scheduling.Resources{CPU: server.Spec.Resources.CPULimitCores, MemoryMB: int64(server.Spec.Resources.MemoryLimitMB)}, reserved,
	)
	return err
}

func scoreLeastAllocated(node domain.ComputeNode, existing []domain.GameServer) int64 {
	runningOnNode := 0
	allocatedMem := int64(0)
	for _, s := range existing {
		if s.NodeID == node.ID {
			runningOnNode++
			allocatedMem += int64(s.Spec.Resources.MemoryLimitMB)
		}
	}

	freeMem := node.MemoryTotalMB - allocatedMem
	if freeMem < 0 {
		freeMem = 0
	}

	return freeMem - int64(runningOnNode*512)
}

// WithLocalArchitecture supplies the control plane's current Docker probe.
func (s *Scheduler) WithLocalArchitecture(read func() string) *Scheduler {
	s.localArchitecture = read
	return s
}
