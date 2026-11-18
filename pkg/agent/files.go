package agent

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	run "github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

// authenticateFileOrNodeRequest validates either a signed FileTicket query parameter (?ticket=...)
// or the node-agent Bearer token, returning any decoded FileTicketClaims.
func (a *Agent) authenticateFileOrNodeRequest(r *http.Request, tenant, instance string) (*run.FileTicketClaims, bool) {
	tokens := a.acceptedTokens()
	if len(tokens) == 0 {
		return nil, false
	}
	if ticket := strings.TrimSpace(r.URL.Query().Get("ticket")); ticket != "" {
		for _, secret := range tokens {
			claims, err := run.VerifyFileTicket(secret, ticket, time.Now())
			if err != nil {
				continue
			}
			if claims.Tenant != tenant || claims.Instance != instance {
				return nil, false
			}
			if claims.NodeName != "" && a.cfg != nil && a.cfg.NodeName != "" && claims.NodeName != a.cfg.NodeName {
				return nil, false
			}
			return claims, true
		}
		return nil, false
	}
	const prefix = "Bearer "
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, prefix) {
		got := strings.TrimPrefix(h, prefix)
		for _, secret := range tokens {
			if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1 {
				return nil, true
			}
		}
	}
	return nil, false
}

var errFileTicketBinding = errors.New("file ticket does not match the current instance binding")

func validateFileTicketBinding(inst *corev1.Instance, claims *run.FileTicketClaims) error {
	if claims == nil {
		return nil
	}
	c := inst.Status.ResourceControl
	b := c.CurrentAttempt
	if claims.InstanceUID == "" || claims.InstanceUID != inst.Metadata.UID || claims.NodeName != b.NodeName || claims.NodeUID == "" || claims.NodeUID != b.NodeUID || claims.AttemptID == "" || claims.AttemptID != b.ID || claims.Epoch <= 0 || claims.Epoch != b.Epoch || claims.ApprovedRevision <= 0 || claims.ApprovedRevision != c.ApprovedRevision {
		return errFileTicketBinding
	}
	return nil
}

func (a *Agent) beginFileAccess(ctx context.Context, tenant, name, requestedUID string, claims *run.FileTicketClaims) (*corev1.Instance, func(), error) {
	inst, _, err := a.currentInstanceForAccess(ctx, tenant, name)
	if err != nil {
		return nil, nil, err
	}
	if requestedUID != "" && requestedUID != inst.Metadata.UID {
		return nil, nil, errFileTicketBinding
	}
	if err := validateFileTicketBinding(inst, claims); err != nil {
		return nil, nil, err
	}
	gate := a.gateForUID(inst.Metadata.UID)
	gateCtx, cancel := a.operationContext(ctx)
	defer cancel()
	release, err := gate.acquire(gateCtx, false)
	if err != nil {
		return nil, nil, err
	}
	// Re-read after waiting: the gate is local coordination, not an authority
	// substitute. The primary state also restores rejection after agent restart.
	latest, _, err := a.currentInstanceForAccess(ctx, tenant, name)
	if err == nil && !sameExecution(inst, latest) {
		err = errFileTicketBinding
	}
	if err == nil {
		err = validateFileTicketBinding(latest, claims)
	}
	if err != nil {
		release()
		return nil, nil, err
	}
	return latest, release, nil
}

// rateLimitedReader wraps an io.Reader with golang.org/x/time/rate token-bucket pacing so large
// file uploads or S3 backup/restore streams cannot saturate the host NIC or disk IO.
type rateLimitedReader struct {
	ctx     context.Context
	r       io.Reader
	limiter *rate.Limiter
}

func newRateLimitedReader(ctx context.Context, r io.Reader, bytesPerSec int64) io.Reader {
	if bytesPerSec <= 0 {
		return r
	}
	burst := int(min(max(bytesPerSec, 1), 32*1024))
	lim := rate.NewLimiter(rate.Limit(bytesPerSec), burst)
	lim.AllowN(time.Now(), burst)
	return &rateLimitedReader{ctx: ctx, r: r, limiter: lim}
}

func (rl *rateLimitedReader) Read(p []byte) (int, error) {
	if err := rl.ctx.Err(); err != nil {
		return 0, err
	}
	if burst := rl.limiter.Burst(); len(p) > burst {
		p = p[:burst]
	}
	n, err := rl.r.Read(p)
	if n > 0 {
		if waitErr := rl.limiter.WaitN(rl.ctx, n); waitErr != nil {
			return n, waitErr
		}
	}
	return n, err
}

func (rl *rateLimitedReader) Close() error {
	if c, ok := rl.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func validatePresignedHTTPURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid presignedURL: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("presignedURL must use http or https with a valid host")
	}
	return u, nil
}

func newNoRedirectHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errors.New("redirects are not allowed for presigned object storage URLs")
		},
	}
}

func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    code,
		"message": msg,
	})
}

func (a *Agent) removeInstanceWorkDir(uid, tenant, name string) error {
	return run.RemoveInstanceWorkDir(a.cfg.WorkDir, uid, tenant, name)
}

func (a *Agent) cleanupInstanceStorage(ctx context.Context, uid, tenant, name string, rc *corev1.RuntimeClass) error {
	if err := a.removeInstanceWorkDir(uid, tenant, name); err != nil {
		return err
	}
	if strings.TrimSpace(a.cfg.WorkDir) == "" || rc == nil {
		return nil
	}
	quota, err := run.ForRuntimeClass(a.cfg.WorkDir, rc)
	if err != nil {
		return err
	}
	return quota.Release(ctx, uid)
}

func removeString(slice []string, target string) []string {
	var out []string
	for _, item := range slice {
		if item != target {
			out = append(out, item)
		}
	}
	return out
}

// handleFiles adapts standard http.ServeMux path values to handleInstanceFiles.
func (a *Agent) handleFiles(w http.ResponseWriter, r *http.Request) {
	a.handleInstanceFiles(w, r, r.PathValue("tenant"), r.PathValue("name"))
}

// handleInstanceFiles serves /apis/core.gamepanel.io/v1/tenants/{tenant}/instances/{name}/files
// directly on the node-agent using os.Root isolation and centralized ticket authorization.
func (a *Agent) handleInstanceFiles(w http.ResponseWriter, r *http.Request, tenant, name string) {
	setCORSHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := metav1.ValidateSingleSegmentName("tenant", tenant); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := metav1.ValidateSingleSegmentName("instance", name); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	claims, ok := a.authenticateFileOrNodeRequest(r, tenant, name)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized: valid node token or signed file ticket required")
		return
	}
	ctx, cancel, stop := bindTransferDeadline(w, r)
	defer cancel()
	defer stop()
	r = r.WithContext(ctx)
	inst, release, err := a.beginFileAccess(ctx, tenant, name, strings.TrimSpace(r.URL.Query().Get("uid")), claims)
	if err != nil {
		writeFileAccessError(w, err)
		return
	}
	defer release()
	root, _, err := run.OpenInstanceRoot(a.cfg.WorkDir, inst.Metadata.UID, tenant, name, false)
	if err != nil {
		status := http.StatusBadRequest
		if os.IsNotExist(err) {
			status = http.StatusNotFound
		}
		writeJSONError(w, status, err.Error())
		return
	}
	defer func() { _ = root.Close() }()
	a.dispatchInstanceFileMethod(w, r, ctx, tenant, name, inst, root, claims, r.URL.Query().Get("path"))
}

func bindTransferDeadline(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, func() bool) {
	ctx, cancel := context.WithTimeout(r.Context(), agentTransferTimeout)
	controller := http.NewResponseController(w)
	deadline, _ := ctx.Deadline()
	_ = controller.SetReadDeadline(deadline)
	_ = controller.SetWriteDeadline(deadline)
	stop := context.AfterFunc(ctx, func() {
		_ = controller.SetReadDeadline(time.Now())
		_ = controller.SetWriteDeadline(time.Now())
		_ = r.Body.Close()
	})
	return ctx, cancel, stop
}

func writeFileAccessError(w http.ResponseWriter, err error) {
	status := http.StatusNotFound
	if errors.Is(err, errFileTicketBinding) {
		status = http.StatusForbidden
	} else if errors.Is(err, errInstanceClosing) {
		status = http.StatusGone
	} else if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	}
	writeJSONError(w, status, err.Error())
}

func (a *Agent) dispatchInstanceFileMethod(w http.ResponseWriter, r *http.Request, ctx context.Context, tenant, name string, inst *corev1.Instance, root *os.Root, claims *run.FileTicketClaims, reqPath string) {
	switch r.Method {
	case http.MethodGet:
		handleGetInstanceFile(w, r, tenant, name, root, claims, reqPath)
	case http.MethodPut:
		handlePutInstanceFile(w, r, ctx, tenant, name, root, claims, reqPath)
	case http.MethodDelete:
		handleDeleteInstanceFile(w, root, claims, reqPath)
	case http.MethodPost:
		a.handlePostInstanceFile(w, r, tenant, name, inst, root, claims, reqPath)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func handleGetInstanceFile(w http.ResponseWriter, r *http.Request, tenant, name string, root *os.Root, claims *run.FileTicketClaims, reqPath string) {
	if claims != nil {
		if err := claims.AllowsMethodAndAction(http.MethodGet, ""); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		if err := claims.AllowsPath(reqPath, true); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
	}
	info, rel, err := run.StatInRoot(root, reqPath, true)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "file or directory not found")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if info.IsDir() {
		items, err := run.ListEntriesInRoot(root, rel)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		outPath := "/"
		if rel != "." {
			outPath = "/" + rel
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(corev1.InstanceFiles{
			TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"},
			Metadata: metav1.ObjectMeta{Name: name, Tenant: tenant},
			Action:   corev1.FileActionList,
			Path:     outPath,
			Entries:  items,
		})
		return
	}
	if claims != nil && strings.EqualFold(claims.Action, corev1.FileActionList) {
		writeJSONError(w, http.StatusForbidden, `ticket action "list" cannot read file content`)
		return
	}
	if !info.Mode().IsRegular() {
		writeJSONError(w, http.StatusBadRequest, "only regular files can be read")
		return
	}
	f, fileInfo, _, err := run.OpenReadInRoot(root, rel)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(rel)}))
	}
	http.ServeContent(w, r, fileInfo.Name(), fileInfo.ModTime(), f)
}

func handlePutInstanceFile(w http.ResponseWriter, r *http.Request, ctx context.Context, tenant, name string, root *os.Root, claims *run.FileTicketClaims, reqPath string) {
	if r.URL.Query().Get("extract") == "tar.gz" {
		handleExtractArchiveInRoot(w, r, ctx, tenant, name, root, claims, reqPath)
		return
	}
	if claims != nil {
		if err := claims.AllowsMethodAndAction(http.MethodPut, corev1.FileActionWrite); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		if err := claims.AllowsPath(reqPath, false); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
	}
	if _, err := metav1.NormalizeRelativeFilePath(reqPath, false); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	maxBytes, rateLimit, ok := resolveWriteTicketQuotas(w, claims)
	if !ok {
		return
	}
	if r.ContentLength > maxBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("upload exceeds allowed MaxBytes quota (%d bytes)", maxBytes))
		return
	}
	reader := newRateLimitedReader(r.Context(), r.Body, rateLimit)
	written, err := run.WriteStreamInRootContext(ctx, root, reqPath, reader, maxBytes)
	if err != nil {
		writeFileWriteError(w, err, maxBytes, "upload exceeds allowed MaxBytes quota (%d bytes)", "write file failed", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(corev1.InstanceFiles{
		TypeMeta:    metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"},
		Metadata:    metav1.ObjectMeta{Name: name, Tenant: tenant},
		Action:      corev1.FileActionWrite,
		Path:        reqPath,
		SizeBytes:   written,
		CompletedAt: &now,
	})
}

func handleExtractArchiveInRoot(w http.ResponseWriter, r *http.Request, ctx context.Context, tenant, name string, root *os.Root, claims *run.FileTicketClaims, reqPath string) {
	if claims != nil {
		if err := claims.AllowsMethodAndAction(http.MethodPut, corev1.FileActionWrite); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		if err := claims.AllowsPath(reqPath, true); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
	}
	rel, err := metav1.NormalizeRelativeFilePath(reqPath, true)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	maxBytes, rateLimit, ok := resolveWriteTicketQuotas(w, claims)
	if !ok {
		return
	}
	targetRoot := root
	if rel != "." {
		if _, err := run.EnsureSafeDirInRoot(root, rel); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		sub, err := root.OpenRoot(filepath.FromSlash(rel))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		defer func() { _ = sub.Close() }()
		targetRoot = sub
	}
	if err := run.ExtractTarGzToRootContext(ctx, targetRoot, newRateLimitedReader(r.Context(), r.Body, rateLimit), maxBytes); err != nil {
		writeFileWriteError(w, err, maxBytes, "archive exceeds allowed MaxBytes quota (%d bytes)", "extract archive failed", http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(corev1.InstanceFiles{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"}, Metadata: metav1.ObjectMeta{Name: name, Tenant: tenant},
		Action: corev1.FileActionWrite, Path: reqPath, CompletedAt: &now,
	})
}

func resolveWriteTicketQuotas(w http.ResponseWriter, claims *run.FileTicketClaims) (int64, int64, bool) {
	maxBytes := int64(512 << 20)
	var rateLimit int64
	if claims != nil {
		var err error
		maxBytes, rateLimit, err = claims.ResolveQuotas(0, 0)
		if err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return 0, 0, false
		}
		if maxBytes <= 0 {
			maxBytes = 512 << 20
		}
	}
	return maxBytes, rateLimit, true
}

func isDiskQuotaError(err error) bool {
	return errors.Is(err, syscall.EDQUOT) || errors.Is(err, syscall.ENOSPC)
}

func writeFileWriteError(w http.ResponseWriter, err error, maxBytes int64, limitFmt, prefix string, defaultStatus int) {
	if errors.Is(err, run.ErrFileQuotaExceeded) {
		writeJSONError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf(limitFmt, maxBytes))
		return
	}
	if isDiskQuotaError(err) {
		writeJSONError(w, http.StatusInsufficientStorage, fmt.Sprintf("instance storage quota exceeded: %v", err))
		return
	}
	writeJSONError(w, defaultStatus, fmt.Sprintf("%s: %v", prefix, err))
}

func handleDeleteInstanceFile(w http.ResponseWriter, root *os.Root, claims *run.FileTicketClaims, reqPath string) {
	if claims != nil {
		if err := claims.AllowsMethodAndAction(http.MethodDelete, corev1.FileActionDelete); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		if err := claims.AllowsPath(reqPath, false); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
	}
	if err := run.RemoveAllInRoot(root, reqPath); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Agent) handlePostInstanceFile(w http.ResponseWriter, r *http.Request, tenant, name string, inst *corev1.Instance, root *os.Root, claims *run.FileTicketClaims, reqPath string) {
	var req corev1.InstanceFiles
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if claims != nil {
		if err := claims.AllowsMethodAndAction(http.MethodPost, action); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
	}
	switch action {
	case corev1.FileActionMkdir:
		handleMkdirInstanceFile(w, tenant, name, root, claims, &req, reqPath)
	case corev1.FileActionRename:
		handleRenameInstanceFile(w, tenant, name, root, claims, &req, reqPath)
	case corev1.FileActionBackup, corev1.FileActionRestore:
		a.handleS3TransferAction(w, r, tenant, name, inst, root, claims, &req, action)
	default:
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("unsupported POST file action %q", req.Action))
	}
}

func handleMkdirInstanceFile(w http.ResponseWriter, tenant, name string, root *os.Root, claims *run.FileTicketClaims, req *corev1.InstanceFiles, reqPath string) {
	p := req.Path
	if p == "" {
		p = reqPath
	}
	if claims != nil {
		if err := claims.AllowsPath(p, false); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
	}
	if err := run.MkdirAllInRoot(root, p); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(corev1.InstanceFiles{
		TypeMeta:    metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"},
		Metadata:    metav1.ObjectMeta{Name: name, Tenant: tenant},
		Action:      corev1.FileActionMkdir,
		Path:        p,
		CompletedAt: &now,
	})
}

func handleRenameInstanceFile(w http.ResponseWriter, tenant, name string, root *os.Root, claims *run.FileTicketClaims, req *corev1.InstanceFiles, reqPath string) {
	oldRel := req.Path
	if oldRel == "" {
		oldRel = reqPath
	}
	if claims != nil {
		if err := claims.AllowsPath(oldRel, false); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		if err := claims.AllowsTargetPath(req.TargetPath); err != nil {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
	}
	if err := run.RenameInRoot(root, oldRel, req.TargetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSONError(w, http.StatusNotFound, "source file or directory not found")
			return
		}
		if errors.Is(err, os.ErrExist) {
			writeJSONError(w, http.StatusConflict, "target path already exists")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(corev1.InstanceFiles{
		TypeMeta:    metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"},
		Metadata:    metav1.ObjectMeta{Name: name, Tenant: tenant},
		Action:      corev1.FileActionRename,
		Path:        oldRel,
		TargetPath:  req.TargetPath,
		CompletedAt: &now,
	})
}

func (a *Agent) handleS3TransferAction(w http.ResponseWriter, r *http.Request, tenant, name string, inst *corev1.Instance, root *os.Root, claims *run.FileTicketClaims, req *corev1.InstanceFiles, action string) {
	if claims == nil {
		writeJSONError(w, http.StatusForbidden, action+" requires a dedicated signed ticket bound to the presignedURL")
		return
	}
	if err := claims.AllowsPresignedURL(action, req.PresignedURL); err != nil {
		writeJSONError(w, http.StatusForbidden, err.Error())
		return
	}
	maxBytes, rateLimit, qerr := claims.ResolveQuotas(req.MaxBytes, req.RateLimitBytesPerSec)
	if qerr != nil {
		writeJSONError(w, http.StatusForbidden, qerr.Error())
		return
	}
	req.MaxBytes = maxBytes
	req.RateLimitBytesPerSec = rateLimit
	if action == corev1.FileActionBackup {
		a.handleStreamingS3Backup(w, r, inst, root, req)
		return
	}
	replace := req.ReplaceTarget != nil && *req.ReplaceTarget
	if err := claims.AllowsRestoreMapping(req.SourcePath, req.TargetPath, replace); err != nil {
		writeJSONError(w, http.StatusForbidden, err.Error())
		return
	}
	a.handleStreamingS3Restore(w, r, inst, root, req)
}

// handleStreamingS3Backup archives the instance's configured backup subtree (or full root)
// as a gzipped tarball using os.Root and streams it to the S3 Pre-signed PUT URL.
func (a *Agent) handleStreamingS3Backup(w http.ResponseWriter, r *http.Request, inst *corev1.Instance, root *os.Root, req *corev1.InstanceFiles) {
	if _, err := validatePresignedHTTPURL(req.PresignedURL); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 2 << 30
	}
	tmpDir := filepath.Join(a.cfg.WorkDir, ".tmp")
	if a.cfg.WorkDir == "" {
		tmpDir = os.TempDir()
	} else if err := os.MkdirAll(tmpDir, 0o750); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("prepare backup temp dir: %v", err))
		return
	}
	tmp, err := os.CreateTemp(tmpDir, "gp-backup-*.tar.gz")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("create backup staging file: %v", err))
		return
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	ts := newTaskStreamWriter(w, r)
	a.executeStreamingBackup(ts, r, inst, root, tmp, maxBytes, req)
}

func (a *Agent) executeStreamingBackup(ts *taskStreamWriter, r *http.Request, inst *corev1.Instance, root *os.Root, tmp *os.File, maxBytes int64, req *corev1.InstanceFiles) {
	size, sum, archErr := a.archiveInstanceBackup(r.Context(), ts, inst, root, tmp, maxBytes)
	if archErr != nil {
		if errors.Is(archErr, run.ErrFileQuotaExceeded) {
			ts.fail(http.StatusRequestEntityTooLarge, fmt.Sprintf("backup exceeds allowed MaxBytes quota (%d bytes)", maxBytes))
			return
		}
		ts.fail(http.StatusInternalServerError, fmt.Sprintf("backup archive failed: %v", archErr))
		return
	}
	ts.log(corev1.BackupStageUploading, fmt.Sprintf("存档打包完成（压缩后 %s），准备上传至对象存储...", formatArchiveBytes(size)))
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		ts.fail(http.StatusInternalServerError, fmt.Sprintf("rewind backup staging file: %v", err))
		return
	}
	statusCode, doErr, ok := streamBackupFileToPresignedURL(ts, r, tmp, size, req)
	if !ok || !validateS3BackupUpload(ts, statusCode, doErr) {
		return
	}
	now := time.Now().UTC()
	req.SizeBytes, req.SHA256, req.CompletedAt = size, sum, &now
	ts.log(corev1.BackupStageCompleted, "存档已成功上传至对象存储")
	ts.result(req)
}

func (a *Agent) archiveInstanceBackup(ctx context.Context, ts *taskStreamWriter, inst *corev1.Instance, root *os.Root, tmp *os.File, maxBytes int64) (int64, string, error) {
	includePaths := prepareBackupArchivePaths(inst, root)
	if quiescer, rc := a.resolveBackupQuiescer(inst); quiescer != nil {
		ts.log(corev1.BackupStageStopping, "该应用需先安全停服以确保存档落盘，准备停止服务器...")
		var size int64
		var sum string
		err := quiescer.WithQuiescedWorkload(ctx, inst, rc, func(stage string) {
			emitQuiesceStageWithCopy(ts, stage, "正在安全停止服务器以确保世界数据完整落盘...", 10, "存档打包完成，正在重新启动服务器...", 78)
		}, func(_ string) error {
			ts.log(corev1.BackupStageArchiving, "正在扫描并打包游戏存档文件...")
			ts.progress(corev1.BackupStageArchiving, 15, 0, 0, 0, "正在打包存档")
			var archErr error
			size, sum, archErr = archiveWithThrottledProgress(root, includePaths, tmp, maxBytes, ts)
			return archErr
		})
		var postErr *runtime.PostQuiesceError
		if errors.As(err, &postErr) {
			ts.log(corev1.BackupStageRestarting, fmt.Sprintf("存档已打包完成，后续实例重拉起将由控制器自动重试: %v", postErr.Err))
			return size, sum, nil
		}
		return size, sum, err
	}
	a.flushInstanceBeforeBackup(ctx, ts, inst)
	ts.log(corev1.BackupStageArchiving, "正在扫描并打包游戏存档文件...")
	ts.progress(corev1.BackupStageArchiving, 15, 0, 0, 0, "正在打包存档")
	return archiveWithThrottledProgress(root, includePaths, tmp, maxBytes, ts)
}

func (a *Agent) resolveBackupQuiescer(inst *corev1.Instance) (runtime.MaintenanceQuiescer, *corev1.RuntimeClass) {
	if a == nil || a.drivers == nil || inst == nil || inst.Spec.BackupPolicy == nil {
		return nil, nil
	}
	if !inst.Spec.BackupPolicy.BackupRequiresStop || corev1.EffectiveInstanceState(inst) == corev1.InstanceStateStopped {
		return nil, nil
	}
	rc, err := frozenRuntimeClass(inst)
	if err != nil {
		return nil, nil
	}
	drv, ok := a.drivers.Get(rc.Handler)
	if !ok {
		return nil, nil
	}
	quiescer, _ := drv.(runtime.MaintenanceQuiescer)
	return quiescer, rc
}

func (a *Agent) flushInstanceBeforeBackup(ctx context.Context, ts *taskStreamWriter, inst *corev1.Instance) {
	if a == nil || a.drivers == nil || inst == nil || inst.Spec.BackupPolicy == nil {
		return
	}
	if corev1.EffectiveInstanceState(inst) == corev1.InstanceStateStopped || inst.Status.Phase != corev1.InstancePhaseRunning {
		return
	}
	cmd := strings.TrimSpace(inst.Spec.BackupPolicy.PreBackupCommand)
	if cmd == "" {
		return
	}
	rc, err := frozenRuntimeClass(inst)
	if err != nil {
		return
	}
	drv, ok := a.drivers.Get(rc.Handler)
	if !ok {
		return
	}
	if isDSTSaveFlush(inst, cmd) {
		a.confirmDSTBackupSave(ctx, ts, drv, inst)
		return
	}
	if ts != nil {
		ts.log(corev1.BackupStageArchiving, "正在通知游戏服务端保存最新进度...")
	}
	flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	argv := runtime.BuildConsoleCommandArgv(inst.Spec.BackupPolicy.ConsoleDelivery, cmd)
	if _, err := drv.Exec(flushCtx, inst, runtime.ExecRequest{Command: argv}); err == nil {
		time.Sleep(500 * time.Millisecond)
	}
}

func archiveWithThrottledProgress(root *os.Root, includePaths []string, tmp *os.File, maxBytes int64, ts *taskStreamWriter) (int64, string, error) {
	lastEmit := time.Now()
	return run.ArchivePathsToTarGzWithProgress(root, includePaths, tmp, maxBytes, func(writtenBytes int64, entries int) {
		if ts == nil || !ts.active || time.Since(lastEmit) < 400*time.Millisecond {
			return
		}
		lastEmit = time.Now()
		ts.progress(corev1.BackupStageArchiving, 30, writtenBytes, 0, 0, fmt.Sprintf("正在打包存档（已处理 %d 个文件）", entries))
	})
}

func prepareBackupArchivePaths(inst *corev1.Instance, root *os.Root) []string {
	if inst == nil || inst.Spec.BackupPolicy == nil {
		return nil
	}
	rawSub := strings.TrimSpace(inst.Spec.BackupPolicy.Subtree)
	if rawSub == "" || rawSub == "." || rawSub == "/" {
		return nil
	}
	parts := strings.Split(rawSub, ",")
	var paths []string
	for _, part := range parts {
		sub := strings.TrimSpace(part)
		if sub == "" || sub == "." || sub == "/" {
			continue
		}
		if len(parts) == 1 {
			_ = run.MkdirAllInRoot(root, sub)
		}
		paths = append(paths, sub)
	}
	if len(paths) == 0 {
		return nil
	}
	for _, f := range inst.Spec.Files {
		if p := strings.TrimSpace(f.Path); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

func formatArchiveBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// streamBackupFileToPresignedURL uploads body to the presigned PUT URL and returns the object storage
// HTTP status code. The response body is always drained and closed here.
func streamBackupFileToPresignedURL(ts *taskStreamWriter, r *http.Request, body io.Reader, size int64, req *corev1.InstanceFiles) (int, error, bool) {
	tracked := newProgressReader(body, size, func(processed, total, speed int64) {
		pct := computeTransferPct(40, 58, processed, total)
		ts.progress(corev1.BackupStageUploading, pct, processed, total, speed, "正在上传备份文件")
	})
	streamBody := newRateLimitedReader(r.Context(), tracked, req.RateLimitBytesPerSec)
	putReq, err := http.NewRequestWithContext(r.Context(), http.MethodPut, req.PresignedURL, streamBody)
	if err != nil {
		ts.fail(http.StatusBadRequest, fmt.Sprintf("invalid presignedURL: %v", err))
		return 0, nil, false
	}
	putReq.ContentLength = size
	putReq.Header.Set("Content-Type", "application/gzip")
	resp, doErr := newNoRedirectHTTPClient(30 * time.Minute).Do(putReq)
	if doErr != nil {
		return 0, doErr, true
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil, true
}

func validateS3BackupUpload(ts *taskStreamWriter, statusCode int, doErr error) bool {
	if doErr != nil {
		ts.fail(http.StatusBadGateway, fmt.Sprintf("upload backup to object storage failed: %v", doErr))
		return false
	}
	if statusCode < 200 || statusCode >= 300 {
		ts.fail(http.StatusBadGateway, fmt.Sprintf("object storage returned HTTP %d", statusCode))
		return false
	}
	return true
}

// handleStreamingS3Restore streams a backup archive (.tar.gz or .zip) from the signed S3 Pre-signed GET URL,
// stages & validates entries inside root, and atomically swaps them into place under workload quiescing.
func (a *Agent) handleStreamingS3Restore(w http.ResponseWriter, r *http.Request, inst *corev1.Instance, root *os.Root, req *corev1.InstanceFiles) {
	if _, err := validatePresignedHTTPURL(req.PresignedURL); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 2 << 30 // 2 GiB default safety ceiling
	}
	getReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, req.PresignedURL, nil)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid presignedURL: %v", err))
		return
	}
	ts := newTaskStreamWriter(w, r)
	ts.log(corev1.BackupStageDownloading, "正在连接对象存储并拉取备份归档包...")
	a.executeStreamingRestore(ts, r, inst, root, getReq, maxBytes, req)
}

func (a *Agent) executeStreamingRestore(ts *taskStreamWriter, r *http.Request, inst *corev1.Instance, root *os.Root, getReq *http.Request, maxBytes int64, req *corev1.InstanceFiles) {
	resp, err := newNoRedirectHTTPClient(30 * time.Minute).Do(getReq)
	if err != nil {
		ts.fail(http.StatusBadGateway, fmt.Sprintf("download backup from object storage failed: %v", err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		ts.fail(http.StatusBadGateway, fmt.Sprintf("object storage returned HTTP %d", resp.StatusCode))
		return
	}
	staged, err := stageRestoreFromResponse(r.Context(), ts, root, resp, maxBytes, req)
	if err != nil {
		failRestoreStage(ts, err)
		return
	}
	defer func() { _ = staged.Close() }()
	if err := a.promoteStagedRestore(r.Context(), ts, inst, staged); err != nil {
		failRestoreStage(ts, err)
		return
	}
	now := time.Now().UTC()
	req.CompletedAt = &now
	ts.log(corev1.BackupStageExtracting, "存档文件已校验并替换到服务器工作目录")
	ts.result(req)
}

func stageRestoreFromResponse(ctx context.Context, ts *taskStreamWriter, root *os.Root, resp *http.Response, maxBytes int64, req *corev1.InstanceFiles) (*run.StagedRestore, error) {
	total := max(resp.ContentLength, 0)
	tracked := newProgressReader(resp.Body, total, func(processed, tot, speed int64) {
		pct := computeTransferPct(15, 65, processed, tot)
		ts.progress(corev1.BackupStageDownloading, pct, processed, tot, speed, "正在下载并校验归档包")
	})
	streamReader := newRateLimitedReader(ctx, tracked, req.RateLimitBytesPerSec)
	lastExtractEmit := time.Now()
	opts := run.RestoreOptions{
		SourceSubpath: req.SourcePath,
		TargetSubpath: req.TargetPath,
		ReplaceTarget: req.ReplaceTarget != nil && *req.ReplaceTarget,
		OnProgress: func(extracted int64, entries int) {
			if ts.active && time.Since(lastExtractEmit) >= 400*time.Millisecond {
				lastExtractEmit = time.Now()
				ts.progress(corev1.BackupStageExtracting, 85, extracted, max(total, extracted), 0, fmt.Sprintf("正在校验并暂存文件（已处理 %d 个条目）", entries))
			}
		},
	}
	return run.StageArchiveToRoot(ctx, root, streamReader, maxBytes, opts)
}

func (a *Agent) promoteStagedRestore(ctx context.Context, ts *taskStreamWriter, inst *corev1.Instance, staged *run.StagedRestore) error {
	if inst != nil && a.drivers != nil {
		if rc, err := frozenRuntimeClass(inst); err == nil {
			if drv, ok := a.drivers.Get(rc.Handler); ok {
				if quiescer, ok := drv.(runtime.MaintenanceQuiescer); ok {
					ts.log(corev1.BackupStageExtracting, "归档校验通过，准备安全停服并替换文件...")
					err := quiescer.WithQuiescedWorkload(ctx, inst, rc, func(stage string) {
						emitAgentQuiesceStage(ts, stage)
					}, func(_ string) error {
						ts.progress(corev1.BackupStageExtracting, 93, 0, 0, 0, "正在替换目标目录与恢复受管文件")
						return staged.Promote(ctx)
					})
					var postErr *runtime.PostQuiesceError
					if errors.As(err, &postErr) {
						ts.log(corev1.BackupStageRestarting, fmt.Sprintf("存档文件已完成还原，后续实例重拉起将由控制器自动重试: %v", postErr.Err))
						return nil
					}
					return err
				}
			}
		}
	}
	ts.progress(corev1.BackupStageExtracting, 93, 0, 0, 0, "正在写入还原后的存档文件")
	return staged.Promote(ctx)
}

func emitAgentQuiesceStage(ts *taskStreamWriter, stage string) {
	emitQuiesceStageWithCopy(ts, stage, "正在安全停止服务器以切换存档...", 90, "存档替换完成，正在重新启动服务器...", 97)
}

func emitQuiesceStageWithCopy(ts *taskStreamWriter, stage, stopMsg string, stopPct int32, restartMsg string, restartPct int32) {
	switch stage {
	case runtime.QuiesceStageStopping:
		ts.log(corev1.BackupStageStopping, stopMsg)
		ts.progress(corev1.BackupStageStopping, stopPct, 0, 0, 0, stopMsg)
	case runtime.QuiesceStageRestarting:
		ts.log(corev1.BackupStageRestarting, restartMsg)
		ts.progress(corev1.BackupStageRestarting, restartPct, 0, 0, 0, restartMsg)
	}
}

func failRestoreStage(ts *taskStreamWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, run.ErrFileQuotaExceeded) {
		status = http.StatusRequestEntityTooLarge
	} else if isDiskQuotaError(err) {
		status = http.StatusInsufficientStorage
	}
	ts.fail(status, err.Error())
}
