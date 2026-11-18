package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

func (h *Handler) allocateHostPort(ctx context.Context, excludeInstanceID string) (int, error) {
	servers, err := h.store.ListGameServers(ctx)
	if err != nil {
		return 0, err
	}
	used := map[int]bool{}
	for _, s := range servers {
		if s.ID != excludeInstanceID && s.Spec.Network.HostPort > 0 {
			for _, port := range h.serverHostPorts(s) {
				used[port] = true
			}
		}
	}
	port := 7777
	for port < 65535 {
		if !used[port] {
			return port, nil
		}
		port++
	}
	return 0, fmt.Errorf("no available host port in range 7777-65535")
}

func (h *Handler) serverJoinInfo(server domain.GameServer) domain.ServerJoinInfo {
	var info domain.ServerJoinInfo
	gameProvider, ok := h.provider.Get(server.ProviderKey)
	if ok {
		if joinProvider, ok := gameProvider.(provider.JoinInfoProvider); ok {
			info = joinProvider.JoinInfo(server)
		} else {
			info = defaultJoinInfo(server)
		}
	} else {
		info = defaultJoinInfo(server)
	}

	if server.NodeID != "" && server.NodeID != "node-local" {
		node, err := h.store.GetComputeNode(context.Background(), server.NodeID)
		if err == nil {
			if strings.TrimSpace(node.PublicIP) != "" {
				info.Address = strings.TrimSpace(node.PublicIP)
				return info
			} else if strings.TrimSpace(node.Host) != "" && node.Host != "0.0.0.0" {
				info.Address = strings.TrimSpace(node.Host)
				return info
			}
		}
	}

	h.applyPublicHostToJoinInfo(&info)
	return info
}

func (h *Handler) resolvePublicHost() string {
	host, err := h.store.GetSetting(context.Background(), "publicHost")
	if err == nil && strings.TrimSpace(host) != "" {
		return strings.TrimSpace(host)
	}
	if strings.TrimSpace(h.cfg.PublicHost) != "" {
		return strings.TrimSpace(h.cfg.PublicHost)
	}
	return "127.0.0.1"
}

func (h *Handler) resolveLocale(ctx context.Context) string {
	locale, err := h.store.GetSetting(ctx, "locale")
	if err == nil {
		locale = strings.TrimSpace(locale)
		if locale == "zh" || locale == "en" {
			return locale
		}
	}
	return "zh"
}

func (h *Handler) applyPublicHostToJoinInfo(info *domain.ServerJoinInfo) {
	host := h.resolvePublicHost()
	if host == "" || host == info.Address {
		return
	}
	old := info.Address
	info.Address = host
	info.InviteText = strings.ReplaceAll(info.InviteText, old+":"+fmt.Sprintf("%d", info.Port), host+":"+fmt.Sprintf("%d", info.Port))
	info.InviteText = strings.ReplaceAll(info.InviteText, old, host)
}

func defaultJoinInfo(server domain.GameServer) domain.ServerJoinInfo {
	port := server.Spec.Network.HostPort
	if port == 0 {
		port = server.Spec.Network.Port
	}
	address := "127.0.0.1"
	invite := fmt.Sprintf("Join %s at %s:%d", server.Name, address, port)
	return domain.ServerJoinInfo{
		Address:    address,
		Port:       port,
		InviteText: invite,
	}
}

func (h *Handler) resolveHostPort(ctx context.Context, requested int, excludeInstanceID string) (int, error) {
	if requested == 0 {
		return h.allocateHostPort(ctx, excludeInstanceID)
	}
	if requested < 1024 || requested > 65535 {
		return 0, fmt.Errorf("external port must be between 1024 and 65535")
	}
	if err := h.ensureHostPortAvailable(ctx, requested, excludeInstanceID); err != nil {
		return 0, err
	}
	return requested, nil
}

func (h *Handler) ensureHostPortAvailable(ctx context.Context, hostPort int, excludeInstanceID string) error {
	servers, err := h.store.ListGameServers(ctx)
	if err != nil {
		return err
	}
	for _, server := range servers {
		if server.ID != excludeInstanceID {
			for _, port := range h.serverHostPorts(server) {
				if port == hostPort {
					return fmt.Errorf("external port %d is already used", hostPort)
				}
			}
		}
	}
	return nil
}

func (h *Handler) serverHostPorts(server domain.GameServer) []int {
	ports := []int{server.Spec.Network.HostPort}
	if h.provider == nil {
		return ports
	}
	p, ok := h.provider.Get(server.ProviderKey)
	if !ok {
		return ports
	}
	rp, ok := p.(provider.ResourceRuntimeProvider)
	if !ok {
		return ports
	}
	config, err := rp.RuntimeConfigForResource(server)
	if err != nil {
		return ports
	}
	for _, port := range config.AdditionalPorts {
		ports = append(ports, server.Spec.Network.HostPort+port-config.Port)
	}
	for _, port := range config.PortBindings {
		ports = append(ports, server.Spec.Network.HostPort+port.Port-config.Port)
	}
	return ports
}

// Reserve every port in the provider's network plan, including adjacent ports.
// The existing caller lock serializes creation/config changes with maintenance.
func (h *Handler) resolveProviderHostPort(ctx context.Context, requested int, server domain.GameServer) (int, error) {
	if requested != 0 && (requested < 1024 || requested > 65535) {
		return 0, fmt.Errorf("external port must be between 1024 and 65535")
	}
	first := requested
	if first == 0 {
		first = 7777
	}
	servers, err := h.store.ListGameServers(ctx)
	if err != nil {
		return 0, err
	}
	used := map[int]bool{}
	for _, existing := range servers {
		if existing.ID == server.ID {
			continue
		}
		for _, port := range h.serverHostPorts(existing) {
			used[port] = true
		}
	}
	server.Spec.Network.HostPort = 0
	offsets := h.serverHostPorts(server)
	for candidate := first; candidate <= 65535; candidate++ {
		available := true
		for _, offset := range offsets {
			port := candidate + offset
			if port < 1024 || port > 65535 {
				available = false
				break
			}
			if used[port] {
				if requested != 0 {
					return 0, fmt.Errorf("external port %d is already used", port)
				}
				available = false
				break
			}
		}
		if available {
			return candidate, nil
		}
		if requested != 0 {
			return 0, fmt.Errorf("external port range exceeds 65535")
		}
	}
	return 0, fmt.Errorf("no available host port range")
}
