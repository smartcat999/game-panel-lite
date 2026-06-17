package safety

import (
	"fmt"
	"path/filepath"
	"strings"
)

func SafeFileName(name string, allowed ...string) (string, error) {
	base := filepath.Base(name)
	if base != name || strings.Contains(base, "..") || base == "." || base == "" {
		return "", fmt.Errorf("invalid file name")
	}
	ext := strings.ToLower(filepath.Ext(base))
	for _, item := range allowed {
		if ext == item {
			return base, nil
		}
	}
	return "", fmt.Errorf("unsupported file extension")
}

func SafeJoin(root string, parts ...string) (string, error) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	all := append([]string{cleanRoot}, parts...)
	target, err := filepath.Abs(filepath.Join(all...))
	if err != nil {
		return "", err
	}
	if target != cleanRoot && !strings.HasPrefix(target, cleanRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes data root")
	}
	return target, nil
}
