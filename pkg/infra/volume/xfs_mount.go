package volume

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func parseMountInfoLine(line string) (fsRoot, mounted, mountOpts, fsType, superOpts string, ok bool, err error) {
	parts := strings.SplitN(line, " - ", 2)
	if len(parts) != 2 {
		return "", "", "", "", "", false, nil
	}
	left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
	if len(left) < 6 || len(right) < 3 {
		return "", "", "", "", "", false, errors.New("invalid Linux mountinfo")
	}
	mounted = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`).Replace(left[4])
	return left[3], mounted, left[5], right[0], right[2], true, nil
}

func isEnforcedXFSOptions(fsRoot, mountOpts, fsType, superOpts string) bool {
	options := "," + superOpts + ","
	hasProj := strings.Contains(options, ",prjquota,") || strings.Contains(options, ",pquota,")
	return fsType == "xfs" && fsRoot == "/" && hasProj && !strings.Contains(options, "noenforce") && !strings.Contains(","+mountOpts+",", ",ro,")
}

// verifyXFSMountInfo parses a read-only Linux mountinfo snapshot for a volume path.
func verifyXFSMountInfo(data []byte, expected, volume string) error {
	found := false
	longest := ""
	for _, line := range strings.Split(string(data), "\n") {
		fsRoot, mounted, mountOpts, fsType, superOpts, ok, err := parseMountInfoLine(line)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if pathWithin(volume, mounted) && mounted != volume {
			return errors.New("instance quota tree contains another host mount")
		}
		if pathWithin(mounted, volume) && len(mounted) > len(longest) {
			longest = mounted
		}
		if mounted == expected {
			if !isEnforcedXFSOptions(fsRoot, mountOpts, fsType, superOpts) {
				return fmt.Errorf("configured XFS mount must already enforce project quotas: %w", ErrStorageQuotaUnsupported)
			}
			found = true
		}
	}
	if !found || filepath.Clean(longest) != expected {
		return fmt.Errorf("volume is not on the configured XFS mount: %w", ErrStorageQuotaUnsupported)
	}
	return nil
}

// verifyXFSMountRoot verifies that expected mountpoint itself is an enforcing RW XFS mount.
func verifyXFSMountRoot(data []byte, expected string) error {
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		fsRoot, mounted, mountOpts, fsType, superOpts, ok, err := parseMountInfoLine(line)
		if err != nil {
			return err
		}
		if !ok || mounted != expected {
			continue
		}
		if !isEnforcedXFSOptions(fsRoot, mountOpts, fsType, superOpts) {
			return fmt.Errorf("configured XFS mount must already enforce project quotas: %w", ErrStorageQuotaUnsupported)
		}
		found = true
	}
	if !found {
		return fmt.Errorf("volume is not on the configured XFS mount: %w", ErrStorageQuotaUnsupported)
	}
	return nil
}
