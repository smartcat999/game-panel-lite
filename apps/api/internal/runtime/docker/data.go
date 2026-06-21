package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/go-connections/nat"
)

func writeDataFile(dataDir string, name string, content string) error {
	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("invalid container data file path: %s", name)
	}
	target := filepath.Join(dataDir, clean)
	if err := ensureRuntimeWritableDir(filepath.Dir(target)); err != nil {
		return err
	}
	if err := os.WriteFile(target, []byte(content), 0o666); err != nil {
		return err
	}
	return os.Chmod(target, 0o666)
}

func dataBinds(dataDir string, mounts []string) ([]string, error) {
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}
	if len(mounts) == 0 {
		mounts = []string{"/data"}
	}
	binds := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		if mount == "" {
			continue
		}
		hostPath, containerPath, err := dataBindPaths(dataDir, mount)
		if err != nil {
			return nil, err
		}
		binds = append(binds, fmt.Sprintf("%s:%s", hostPath, containerPath))
	}
	return binds, nil
}

func prepareDataMounts(dataDir string, mounts []string) error {
	if len(mounts) == 0 {
		return nil
	}
	for _, mount := range mounts {
		if mount == "" {
			continue
		}
		hostPath, _, err := dataBindPaths(dataDir, mount)
		if err != nil {
			return err
		}
		if _, err := os.Stat(hostPath); err == nil {
			if err := ensureRuntimeWritableTree(hostPath); err != nil {
				return err
			}
			continue
		}
		if filepath.Ext(hostPath) != "" {
			if err := ensureRuntimeWritableDir(filepath.Dir(hostPath)); err != nil {
				return err
			}
			file, err := os.OpenFile(hostPath, os.O_CREATE, 0o666)
			if err != nil {
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			if err := os.Chmod(hostPath, 0o666); err != nil {
				return err
			}
			continue
		}
		if err := ensureRuntimeWritableDir(hostPath); err != nil {
			return err
		}
	}
	return nil
}

func ensureRuntimeWritableTree(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ensureRuntimeWritablePath(path)
	}
	return filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return os.Chmod(current, 0o777)
		}
		return os.Chmod(current, 0o666)
	})
}

func ensureRuntimeWritablePath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return os.Chmod(path, 0o777)
	}
	return os.Chmod(path, 0o666)
}

func ensureRuntimeWritableDir(path string) error {
	if err := os.MkdirAll(path, 0o777); err != nil {
		return err
	}
	return os.Chmod(path, 0o777)
}

func dataBindPaths(dataDir string, mount string) (string, string, error) {
	hostPath := dataDir
	containerPath := mount
	if host, container, ok := strings.Cut(mount, ":"); ok {
		cleanHost := filepath.Clean(strings.TrimSpace(host))
		if cleanHost == "." || cleanHost == ".." || filepath.IsAbs(cleanHost) || strings.HasPrefix(cleanHost, ".."+string(filepath.Separator)) {
			return "", "", fmt.Errorf("invalid data mount host path: %s", host)
		}
		hostPath = filepath.Join(dataDir, cleanHost)
		containerPath = container
	}
	if strings.TrimSpace(containerPath) == "" || !strings.HasPrefix(containerPath, "/") {
		return "", "", fmt.Errorf("invalid data mount container path: %s", containerPath)
	}
	baseAbs, err := filepath.Abs(dataDir)
	if err != nil {
		return "", "", err
	}
	hostAbs, err := filepath.Abs(hostPath)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(baseAbs, hostAbs)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("data mount host path escapes data directory: %s", mount)
	}
	return hostAbs, containerPath, nil
}

func natPortMap(containerPort int, hostPort int, protocol string) nat.PortMap {
	p := nat.Port(fmt.Sprintf("%d/%s", containerPort, normalizePortProtocol(protocol)))
	return nat.PortMap{p: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", hostPort)}}}
}

func natPortSet(containerPort int, protocol string) nat.PortSet {
	return nat.PortSet{nat.Port(fmt.Sprintf("%d/%s", containerPort, normalizePortProtocol(protocol))): struct{}{}}
}

func normalizePortProtocol(protocol string) string {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "udp":
		return "udp"
	default:
		return "tcp"
	}
}
