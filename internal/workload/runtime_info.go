package workload

import "strings"

// RuntimeInfo describes the Docker daemon, which may run on a different host
// or architecture than the Agent process.
type RuntimeInfo struct {
	Version           string
	RunningContainers int
	Architecture      string
}

func NormalizeArchitecture(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "amd64", "x86_64":
		return "amd64"
	case "arm64", "aarch64":
		return "arm64"
	case "arm", "armv7l":
		return "arm"
	case "386", "i386", "i686":
		return "386"
	case "ppc64le", "s390x", "riscv64":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}
