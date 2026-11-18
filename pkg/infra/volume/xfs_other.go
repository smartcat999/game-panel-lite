//go:build !linux

package volume

import (
	"context"
	"fmt"
	"os"
)

type nativeXFSQuotaPlatform struct{}

func (nativeXFSQuotaPlatform) Probe(context.Context, string, string) error {
	return fmt.Errorf("XFS project quotas require Linux: %w", ErrStorageQuotaUnsupported)
}

func (nativeXFSQuotaPlatform) ProbeMount(context.Context, string) error {
	return fmt.Errorf("XFS project quotas require Linux: %w", ErrStorageQuotaUnsupported)
}

func (nativeXFSQuotaPlatform) SyncMount(string) error {
	return fmt.Errorf("XFS project quotas require Linux: %w", ErrStorageQuotaUnsupported)
}

func (nativeXFSQuotaPlatform) Lock(context.Context, *os.File) (func(), error) {
	return nil, fmt.Errorf("XFS project quota locking requires Linux: %w", ErrStorageQuotaUnsupported)
}
