package network

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

const DefaultHostPortDriverName = corev1.DefaultHostPortDriverName

func init() {
	Register(DefaultHostPortDriverName, NewHostPortDriver)
}

// HostPortDriver implements Kubernetes-style hostPort & dynamic node port-pool allocation.
type HostPortDriver struct {
	portRange PortRange
}

// NewHostPortDriver constructs a HostPortDriver with the configured dynamic port range.
func NewHostPortDriver(cfg *Config) (Driver, error) {
	pr := DefaultPortRange()
	if cfg != nil {
		c := *cfg
		c.Default()
		pr = PortRange{
			Min:       c.PortRangeMin,
			Max:       c.PortRangeMax,
			MaxProbes: c.MaxPortProbes,
		}
	}
	if err := pr.Validate(); err != nil {
		return nil, err
	}
	return &HostPortDriver{portRange: pr}, nil
}

// NewHostPortDriverWithRange constructs a HostPortDriver from an explicit PortRange.
func NewHostPortDriverWithRange(pr PortRange) (*HostPortDriver, error) {
	if err := pr.Validate(); err != nil {
		return nil, err
	}
	return &HostPortDriver{portRange: pr}, nil
}

func (d *HostPortDriver) Handler() string { return DefaultHostPortDriverName }

func (d *HostPortDriver) PortRange() PortRange { return d.portRange }

// AssignPorts allocates concrete host ports for requests using claim.
// Fixed requests are claimed first so a dynamic request cannot steal another requested
// fixed port in the same placement. The returned slice preserves canonical request order.
func (d *HostPortDriver) AssignPorts(attemptID string, requests []corev1.PortRequest, claim ClaimFunc) ([]corev1.PortRequest, error) {
	return AssignPortsInRange(d.portRange, attemptID, requests, claim)
}

// ValidateExecutionPorts verifies that the runtime's requested ports are concrete,
// unique per protocol, and match the approved port assignments from the node store.
func (d *HostPortDriver) ValidateExecutionPorts(requested, approved []corev1.InstancePort) error {
	return ValidateExecutionPorts(requested, approved)
}

// DynamicStart computes the deterministic probe starting port for (attemptID, index) within pr.
func DynamicStart(pr PortRange, attemptID string, index int) int32 {
	digest := sha256.Sum256([]byte(attemptID + "\x00" + strconv.Itoa(index)))
	width := uint64(pr.Width())
	return pr.Min + int32(binary.BigEndian.Uint64(digest[:8])%width)
}

// VisitCandidates bounds every dynamic port search within pr, including exhausted ranges.
func VisitCandidates(pr PortRange, req corev1.PortRequest, start int32, try func(int32) (bool, error)) (int32, error) {
	if req.PublishedPort != 0 {
		ok, err := try(req.PublishedPort)
		if err != nil {
			return 0, err
		}
		if ok {
			return req.PublishedPort, nil
		}
		return 0, &corev1.Error{Code: corev1.AdmissionDenied, Detail: "fixed port is already owned"}
	}
	width := pr.Width()
	for n := 0; n < pr.MaxProbes && int32(n) < width; n++ {
		port := pr.Min + (start-pr.Min+int32(n))%width
		ok, err := try(port)
		if err != nil {
			return 0, err
		}
		if ok {
			return port, nil
		}
	}
	return 0, &corev1.Error{Code: corev1.AdmissionDenied, Detail: "bounded dynamic port search exhausted"}
}

// AssignPortsInRange allocates concrete host ports within pr.
func AssignPortsInRange(pr PortRange, attemptID string, requests []corev1.PortRequest, claim ClaimFunc) ([]corev1.PortRequest, error) {
	out := make([]corev1.PortRequest, len(requests))
	for _, fixed := range []bool{true, false} {
		for n, request := range requests {
			if (request.PublishedPort != 0) != fixed {
				continue
			}
			port, err := VisitCandidates(pr, request, DynamicStart(pr, attemptID, n), func(port int32) (bool, error) {
				return claim(request, port)
			})
			if err != nil {
				return nil, err
			}
			request.PublishedPort = port
			out[n] = request
		}
	}
	return out, nil
}

// NormalizedProtocol normalizes a port protocol to uppercase ("TCP" by default).
func NormalizedProtocol(protocol string) string {
	protocol = strings.ToUpper(strings.TrimSpace(protocol))
	if protocol == "" {
		return corev1.ProtocolTCP
	}
	return protocol
}

// SameExecutionPorts reports whether left and right contain the exact same multiset of ports.
func SameExecutionPorts(left, right []corev1.InstancePort) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[corev1.InstancePort]int, len(left))
	for _, p := range left {
		p.Protocol = NormalizedProtocol(p.Protocol)
		counts[p]++
	}
	for _, p := range right {
		p.Protocol = NormalizedProtocol(p.Protocol)
		if counts[p] == 0 {
			return false
		}
		counts[p]--
	}
	return true
}

// ValidateExecutionPorts verifies concrete runtime ports against approved assignments.
func ValidateExecutionPorts(requested, approved []corev1.InstancePort) error {
	if len(requested) > corev1.MaxPortsPerPlacement {
		return fmt.Errorf("runtime ports exceed the admitted assignment limit")
	}
	seen := make(map[string]bool, len(requested))
	for _, port := range requested {
		if port.PublishedPort < 1 || port.PublishedPort > 65535 || port.TargetPort < 1 || port.TargetPort > 65535 {
			return fmt.Errorf("runtime ports require concrete admitted assignments")
		}
		proto := NormalizedProtocol(port.Protocol)
		if proto != corev1.ProtocolTCP && proto != corev1.ProtocolUDP {
			return fmt.Errorf("unsupported runtime port protocol %q", port.Protocol)
		}
		key := proto + "/" + strconv.Itoa(port.PublishedPort)
		if seen[key] {
			return fmt.Errorf("duplicate admitted runtime port %s", key)
		}
		seen[key] = true
	}
	if !SameExecutionPorts(requested, approved) {
		return fmt.Errorf("runtime ports differ from the approved assignments")
	}
	return nil
}
