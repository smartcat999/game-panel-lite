package driver

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
)

const SpeedSampleInterval = 400 * time.Millisecond

var steamProgressRegex = regexp.MustCompile(`progress:\s*([0-9]+(?:\.[0-9]+)?)\s*\(\s*(\d+)\s*/\s*(\d+)\s*\)`)

// ByteSpeedTracker computes smoothed transfer byte-rate samples across progress updates.
type ByteSpeedTracker struct {
	mu       sync.Mutex
	lastAt   time.Time
	lastByte int64
	speed    int64
}

// Update records cumulative bytes transferred and returns the latest throughput estimate in bytes/sec.
func (s *ByteSpeedTracker) Update(currentBytes int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.lastAt.IsZero() {
		s.lastAt = now
		s.lastByte = currentBytes
		return 0
	}
	elapsed := now.Sub(s.lastAt)
	if elapsed < SpeedSampleInterval {
		return s.speed
	}
	delta := currentBytes - s.lastByte
	if delta >= 0 && elapsed > 0 {
		s.speed = int64(float64(delta) / elapsed.Seconds())
	}
	s.lastAt = now
	s.lastByte = currentBytes
	return s.speed
}

// ParseSteamProgress parses SteamCMD progress lines into percent, completed bytes, total bytes, and match flag.
func ParseSteamProgress(line string) (float64, int64, int64, bool) {
	m := steamProgressRegex.FindStringSubmatch(line)
	if len(m) != 4 {
		return 0, 0, 0, false
	}
	pct, err1 := strconv.ParseFloat(m[1], 64)
	done, err2 := strconv.ParseInt(m[2], 10, 64)
	total, err3 := strconv.ParseInt(m[3], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return pct, done, total, true
}

// ScalePercent maps done/total linearly into the [from, to] integer percentage range.
func ScalePercent(done, total int64, from, to int32) int32 {
	if total <= 0 {
		return from
	}
	pct := from + int32(float64(to-from)*float64(done)/float64(total))
	if pct < from {
		return from
	}
	if pct > to {
		return to
	}
	return pct
}

// ArchiveTreeTarGz packs root into dst as a compressed tar.gz archive, skipping transient SteamCMD scratch dirs.
func ArchiveTreeTarGz(root, dst string) (int64, string, error) {
	return ArchiveTreeTarGzWithProgress(context.Background(), root, dst, nil)
}

// ArchiveTreeTarGzWithProgress packs root into dst with cancellation and byte-rate progress reporting.
func ArchiveTreeTarGzWithProgress(ctx context.Context, root, dst string, rep Reporter) (int64, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var totalBytes int64
	if rep != nil {
		var sumErr error
		if totalBytes, sumErr = sumArchiveTreeBytes(ctx, root); sumErr != nil {
			return 0, "", sumErr
		}
	}
	out, err := os.Create(dst)
	if err != nil {
		return 0, "", err
	}
	hasher := sha256.New()
	gz, _ := gzip.NewWriterLevel(io.MultiWriter(out, hasher), gzip.BestSpeed)
	tw := tar.NewWriter(gz)
	prog := &archiveCopyProgress{total: totalBytes, speed: &ByteSpeedTracker{}, rep: rep}
	walkErr := walkArchiveTree(ctx, root, tw, prog)
	twErr := tw.Close()
	gzErr := gz.Close()
	if err := errors.Join(walkErr, twErr, gzErr); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return 0, "", err
	}
	st, statErr := out.Stat()
	closeErr := out.Close()
	if err := errors.Join(statErr, closeErr); err != nil {
		_ = os.Remove(dst)
		return 0, "", err
	}
	return st.Size(), hex.EncodeToString(hasher.Sum(nil)), nil
}

type archiveCopyProgress struct {
	done  int64
	total int64
	speed *ByteSpeedTracker
	rep   Reporter
}

func (p *archiveCopyProgress) advance(n int64) {
	if p == nil || p.rep == nil || p.total <= 0 || n <= 0 {
		return
	}
	p.done += n
	spd := p.speed.Update(p.done)
	pct := ScalePercent(p.done, p.total, 72, 79)
	p.rep.Transfer(registryv1.ArtifactStageArchiving, pct, p.done, p.total, spd, "正在打包校验文件")
}

func skipSteamArchiveDir(slashRel string) bool {
	return strings.HasPrefix(slashRel, "steamapps/downloading") ||
		strings.HasPrefix(slashRel, "steamapps/temp") ||
		strings.HasPrefix(slashRel, "steamapps/shadercache")
}

func sumArchiveTreeBytes(ctx context.Context, root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == root {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		rel, _ := filepath.Rel(root, path)
		if skipSteamArchiveDir(filepath.ToSlash(rel)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			if info, iErr := d.Info(); iErr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}

func walkArchiveTree(ctx context.Context, root string, tw *tar.Writer, prog *archiveCopyProgress) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == root {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		rel, _ := filepath.Rel(root, path)
		slashRel := filepath.ToSlash(rel)
		if skipSteamArchiveDir(slashRel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return writeTarFileEntry(ctx, tw, path, slashRel, d, prog)
	})
}

func writeTarFileEntry(ctx context.Context, tw *tar.Writer, path, slashRel string, d fs.DirEntry, prog *archiveCopyProgress) error {
	info, err := d.Info()
	if err != nil {
		return err
	}
	link := ""
	if info.Mode()&os.ModeSymlink != 0 {
		if link, err = os.Readlink(path); err != nil {
			return nil
		}
	}
	hdr, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return err
	}
	hdr.Name = slashRel
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	return copyTarFileWithContext(ctx, tw, path, prog)
}

func copyTarFileWithContext(ctx context.Context, tw *tar.Writer, path string, prog *archiveCopyProgress) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 256<<10)
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			if _, writeErr := tw.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			prog.advance(int64(n))
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

// FirstStr returns the first trimmed non-empty string in vals.
func FirstStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// FirstSlice returns a if non-empty, otherwise b.
func FirstSlice(a, b []string) []string {
	if len(a) > 0 {
		return a
	}
	return b
}

// FirstTimePtr returns a if non-nil, otherwise b.
func FirstTimePtr(a, b *time.Time) *time.Time {
	if a != nil {
		return a
	}
	return b
}

// MergeDeps converts addonmirror dependencies to registry PackageDependency records with fallback.
func MergeDeps(a []addonmirror.Dependency, fallback []registryv1.PackageDependency) []registryv1.PackageDependency {
	if len(a) == 0 {
		return fallback
	}
	out := make([]registryv1.PackageDependency, 0, len(a))
	for _, d := range a {
		out = append(out, registryv1.PackageDependency{Package: d.ID, Title: d.Title})
	}
	return out
}

// CleanupStagingDir removes a temporary staging directory and prunes its parent directory if empty.
func CleanupStagingDir(stagingDir string) {
	dir := strings.TrimSpace(stagingDir)
	if dir == "" {
		return
	}
	_ = os.RemoveAll(dir)
	parent := filepath.Dir(dir)
	if parent != "" && parent != "." && parent != string(filepath.Separator) {
		_ = os.Remove(parent)
	}
}

// CollectCandidateURLs returns deduplicated non-empty download URLs ordered primary-first.
func CollectCandidateURLs(primary string, fallbacks []string) []string {
	seen := make(map[string]struct{}, 1+len(fallbacks))
	var out []string
	for _, raw := range append([]string{primary}, fallbacks...) {
		clean := strings.TrimSpace(raw)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; !ok {
			seen[clean] = struct{}{}
			out = append(out, clean)
		}
	}
	return out
}
