package instance

import (
	"context"
	"errors"
	"fmt"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
)

func TestHumanizeTaskError(t *testing.T) {
	cases := []struct {
		err      error
		contains string
	}{
		{err: fmt.Errorf("wrap: %w", corev1.AdmissionDenied), contains: "配额已满"},
		{err: fmt.Errorf("wrap: %w", s3.ErrNotFound), contains: "未找到目标备份归档文件"},
		{err: context.DeadlineExceeded, contains: "超时"},
		{err: errors.New("conflict: tenant backup storage quota is zero"), contains: "备份存储配额为 0"},
		{err: errors.New("bad request: platform object storage (s3) is not configured"), contains: "对象存储（S3）尚未配置"},
		{err: errors.New("bad gateway: node-agent files returned HTTP 500"), contains: "计算节点文件/终端服务响应异常"},
	}
	for _, tc := range cases {
		got := HumanizeTaskError(tc.err)
		if got == "" || !containsSubstr(got, tc.contains) {
			t.Fatalf("HumanizeTaskError(%v) = %q, want substring %q", tc.err, got, tc.contains)
		}
	}
}

func TestFormatBackupDisplayName(t *testing.T) {
	autoPath := "backups/demo-mc-01-20261112-040000--e887aae58aa8e5a487e4bbbd2031312d31322030343a3030.tar.gz"
	if got := FormatBackupDisplayName("demo-mc-01", autoPath, ""); got != "自动备份 11-12 04:00" {
		t.Fatalf("expected decoded auto label, got %q", got)
	}
	s3Key := "tenants/admin/instances/demo-mc-01/backup-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef-demo-mc-01-20261112-040000.tar.gz"
	if got := FormatBackupDisplayName("demo-mc-01", s3Key, ""); got != "存档备份 2026-11-12 04:00" {
		t.Fatalf("expected formatted compact timestamp, got %q", got)
	}
	zipPath := "tenants/admin/instances/dst-01/dst-01-20261114-193000--6c6974652d647374.ZIP"
	if got := FormatBackupDisplayName("dst-01", zipPath, ""); got != "lite-dst" {
		t.Fatalf("expected decoded zip label, got %q", got)
	}
}

func containsSubstr(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexString(s, sub) >= 0)
}

func indexString(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
