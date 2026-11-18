package agent

import (
	"net"
	"sort"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
)

var (
	netInterfaces = net.Interfaces
	ifiAddrs      = func(ifi *net.Interface) ([]net.Addr, error) { return ifi.Addrs() }
)

var virtualInterfacePrefixes = []string{
	"docker", "br", "veth", "cni", "flannel", "cali", "weave",
	"virbr", "vmnet", "vnet", "bridge", "dummy", "kube",
}

var vpnInterfacePrefixes = []string{
	"tun", "utun", "tap", "wg", "tailscale",
}

var physicalInterfacePrefixes = []string{
	"eth", "en", "em", "bond", "team", "wl",
}

type interfaceTier int

const (
	tierUnusable interfaceTier = 0
	tierVirtual  interfaceTier = 1
	tierVPN      interfaceTier = 2
	tierGeneric  interfaceTier = 3
	tierPhysical interfaceTier = 4
)

// classifyInterface categorizes a network interface into a priority tier.
func classifyInterface(name string, flags net.Flags, hwAddr net.HardwareAddr) interfaceTier {
	if flags&net.FlagUp == 0 || flags&net.FlagLoopback != 0 {
		return tierUnusable
	}

	lowerName := strings.ToLower(name)
	for _, p := range virtualInterfacePrefixes {
		if strings.HasPrefix(lowerName, p) {
			return tierVirtual
		}
	}

	if flags&net.FlagPointToPoint != 0 {
		return tierVPN
	}
	for _, p := range vpnInterfacePrefixes {
		if strings.HasPrefix(lowerName, p) {
			return tierVPN
		}
	}

	for _, p := range physicalInterfacePrefixes {
		if strings.HasPrefix(lowerName, p) {
			return tierPhysical
		}
	}

	if len(hwAddr) > 0 {
		return tierPhysical
	}

	return tierGeneric
}

// extractIPv4 extracts and validates a unicast IPv4 address from a net.Addr.
func extractIPv4(addr net.Addr) net.IP {
	var ip net.IP
	switch v := addr.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	default:
		return nil
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return nil
	}

	if ip4.IsLoopback() || ip4.IsUnspecified() || ip4.IsMulticast() || ip4.IsLinkLocalUnicast() || ip4.IsLinkLocalMulticast() {
		return nil
	}

	return ip4
}

type addressCandidate struct {
	ip        net.IP
	tier      interfaceTier
	isPrivate bool
	ifaceName string
}

// DetectInternalIP dynamically discovers the most appropriate internal IPv4 address for this node.
// It prioritizes physical interfaces, RFC 1918 private subnets, and excludes virtual container bridges.
// The fallback hierarchy strictly enforces: physical -> generic -> vpn -> 127.0.0.1.
// Virtual interfaces (tierVirtual) are never selected as InternalIP.
func DetectInternalIP() string {
	ifaces, err := netInterfaces()
	if err == nil && len(ifaces) > 0 {
		var candidates []addressCandidate
		for _, ifi := range ifaces {
			tier := classifyInterface(ifi.Name, ifi.Flags, ifi.HardwareAddr)
			if tier == tierUnusable {
				continue
			}

			addrs, err := ifiAddrs(&ifi)
			if err != nil {
				continue
			}

			for _, addr := range addrs {
				ip := extractIPv4(addr)
				if ip == nil {
					continue
				}
				candidates = append(candidates, addressCandidate{
					ip:        ip,
					tier:      tier,
					isPrivate: ip.IsPrivate(),
					ifaceName: ifi.Name,
				})
			}
		}

		if len(candidates) > 0 {
			sort.SliceStable(candidates, func(i, j int) bool {
				if candidates[i].tier != candidates[j].tier {
					return candidates[i].tier > candidates[j].tier
				}
				if candidates[i].isPrivate != candidates[j].isPrivate {
					return candidates[i].isPrivate
				}
				return candidates[i].ifaceName < candidates[j].ifaceName
			})

			best := candidates[0]
			if best.tier > tierVirtual {
				return best.ip.String()
			}
			log.Warn("only virtual network interfaces found on node, falling back to 127.0.0.1", "interface", best.ifaceName)
			return "127.0.0.1"
		}
	}

	log.Warn("no valid non-loopback IPv4 address discovered, falling back to 127.0.0.1")
	return "127.0.0.1"
}
