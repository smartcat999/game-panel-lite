package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
)

func (h *Handler) allocateHostPort(ctx context.Context, excludeInstanceID string, nodeID string) (int, error) {
	servers, err := h.store.ListGameServers(ctx)
	if err != nil {
		return 0, err
	}
	used := map[int]bool{}
	for _, s := range servers {
		if s.ID != excludeInstanceID && s.Spec.Network.HostPort > 0 && s.Spec.DesiredState != domain.DesiredDeleted {
			if nodeID == "" || s.NodeID == nodeID {
				used[s.Spec.Network.HostPort] = true
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

	if server.NodeID != "" {
		node, err := h.store.GetComputeNode(context.Background(), server.NodeID)
		if err == nil {
			if strings.TrimSpace(node.PublicDomain) != "" {
				replaceAddressInJoinInfo(&info, strings.TrimSpace(node.PublicDomain))
				return info
			}
			if server.NodeID != "node-local" {
				if strings.TrimSpace(node.PublicIP) != "" {
					replaceAddressInJoinInfo(&info, strings.TrimSpace(node.PublicIP))
					return info
				} else if strings.TrimSpace(node.Host) != "" && node.Host != "0.0.0.0" {
					replaceAddressInJoinInfo(&info, strings.TrimSpace(node.Host))
					return info
				}
			}
		}
	}

	h.applyPublicHostToJoinInfo(&info)
	return info
}

func replaceAddressInJoinInfo(info *domain.ServerJoinInfo, targetHost string) {
	if targetHost == "" || targetHost == info.Address {
		return
	}
	old := info.Address
	info.Address = targetHost
	info.InviteText = strings.ReplaceAll(info.InviteText, old+":"+fmt.Sprintf("%d", info.Port), targetHost+":"+fmt.Sprintf("%d", info.Port))
	info.InviteText = strings.ReplaceAll(info.InviteText, old, targetHost)
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
	replaceAddressInJoinInfo(info, host)
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

func (h *Handler) resolveHostPort(ctx context.Context, requested int, excludeInstanceID string, nodeID string) (int, error) {
	if requested == 0 {
		return h.allocateHostPort(ctx, excludeInstanceID, nodeID)
	}
	if requested < 1024 || requested > 65535 {
		return 0, fmt.Errorf("external port must be between 1024 and 65535")
	}
	if err := h.ensureHostPortAvailable(ctx, requested, excludeInstanceID, nodeID); err != nil {
		return 0, err
	}
	return requested, nil
}

func (h *Handler) ensureHostPortAvailable(ctx context.Context, hostPort int, excludeInstanceID string, nodeID string) error {
	servers, err := h.store.ListGameServers(ctx)
	if err != nil {
		return err
	}
	if nodeID != "" {
		for _, server := range servers {
			if server.ID != excludeInstanceID && server.NodeID == nodeID && server.Spec.Network.HostPort == hostPort && server.Spec.DesiredState != domain.DesiredDeleted {
				return fmt.Errorf("external port %d is already used on node %s", hostPort, nodeID)
			}
		}
		return nil
	}
	if h.scheduler != nil {
		return nil
	}
	for _, server := range servers {
		if server.ID != excludeInstanceID && server.Spec.Network.HostPort == hostPort && server.Spec.DesiredState != domain.DesiredDeleted {
			return fmt.Errorf("external port %d is already used", hostPort)
		}
	}
	return nil
}
