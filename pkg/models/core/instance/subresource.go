package instance

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
)

func (o *operator) resolveTargetNode(ctx context.Context, inst *corev1.Instance) (string, bool, error) {
	nodeName := strings.TrimSpace(inst.Spec.NodeName)
	if nodeName == "" {
		return "", false, fmt.Errorf("%w: instance %s/%s is not scheduled to any node", models.ErrBadRequest, inst.Metadata.Tenant, inst.Metadata.Name)
	}
	if o.drivers != nil && o.localNodeName != "" && nodeName == o.localNodeName {
		return "", true, nil
	}
	node, err := o.nodeDB.Get(ctx, nodeName)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return "", false, fmt.Errorf("%w: scheduled node %q not found", models.ErrNotFound, nodeName)
		}
		return "", false, fmt.Errorf("lookup node %q: %w", nodeName, err)
	}
	var ip string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeAddressInternalIP && strings.TrimSpace(addr.Address) != "" {
			ip = strings.TrimSpace(addr.Address)
			break
		}
	}
	if ip == "" {
		return "", false, fmt.Errorf("%w: node %q has no InternalIP address reported", models.ErrBadRequest, nodeName)
	}
	if _, _, err := net.SplitHostPort(ip); err == nil {
		return ip, false, nil
	}
	return net.JoinHostPort(ip, strconv.Itoa(o.agentPort)), false, nil
}

func (o *operator) resolveNodeAgentToken(ctx context.Context, nodeName string) string {
	if name := strings.TrimSpace(nodeName); name != "" {
		if kv := cache.Default(); kv != nil {
			if tok, err := kv.Get(ctx, iamv1.CacheKeyPrefixNodeToken+name); err == nil && strings.TrimSpace(tok) != "" {
				return strings.TrimSpace(tok)
			}
		}
	}
	if tok := strings.TrimSpace(o.agentToken); tok != "" {
		return tok
	}
	return "dev-agent-token"
}

func instanceSubresourcePath(tenant, name, sub string) string {
	return fmt.Sprintf("/apis/%s/tenants/%s/instances/%s/%s", corev1.GroupVersion, url.PathEscape(tenant), url.PathEscape(name), sub)
}

func (o *operator) logStreamHTTPClient(follow bool) *http.Client {
	base := o.httpClient
	if base == nil {
		base = &http.Client{Timeout: 30 * time.Second}
	}
	if !follow {
		return base
	}
	cp := *base
	cp.Timeout = 0
	return &cp
}

func (o *operator) StreamLogs(ctx context.Context, tenant, name string, opts ...drvruntime.LogOptions) (io.ReadCloser, error) {
	var logOpts drvruntime.LogOptions
	if len(opts) > 0 {
		logOpts = opts[0]
	}
	existing, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	targetHost, isLocal, err := o.resolveTargetNode(ctx, existing)
	if err != nil {
		return nil, err
	}
	if isLocal {
		drv, err := o.driverForInstance(ctx, existing)
		if err != nil {
			return nil, err
		}
		return drv.Logs(ctx, existing, logOpts)
	}
	remoteURL := "http://" + targetHost + instanceSubresourcePath(tenant, name, "log")
	q := url.Values{}
	if logOpts.Follow {
		q.Set("follow", "true")
	}
	if logOpts.TailLines > 0 {
		q.Set("tailLines", strconv.Itoa(logOpts.TailLines))
	}
	if qs := q.Encode(); qs != "" {
		remoteURL += "?" + qs
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create proxy log request: %w", err)
	}
	if tok := o.resolveNodeAgentToken(ctx, existing.Spec.NodeName); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := o.logStreamHTTPClient(logOpts.Follow).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to connect to node-agent on %s: %v", models.ErrBadGateway, targetHost, err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, mapAgentProxyError(resp, targetHost, tenant, name, "")
	}
	return resp.Body, nil
}

func (o *operator) Exec(ctx context.Context, tenant, name string, req *corev1.InstanceExec) (*corev1.InstanceExec, error) {
	if req == nil || len(req.Command) == 0 {
		return nil, fmt.Errorf("%w: command is required", models.ErrBadRequest)
	}
	existing, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if req.Metadata.UID != "" && req.Metadata.UID != existing.Metadata.UID {
		return nil, fmt.Errorf("%w: exec source UID no longer exists", models.ErrConflict)
	}
	if existing.Metadata.DeletionTimestamp != nil {
		return nil, fmt.Errorf("%w: terminating instance cannot execute commands", models.ErrConflict)
	}
	if corev1.EffectiveInstanceState(existing) == corev1.InstanceStateStopped {
		return nil, fmt.Errorf("%w: cannot execute command in stopped instance %s/%s", models.ErrBadRequest, tenant, name)
	}
	requestCopy := *req
	requestCopy.Metadata = existing.Metadata
	req = &requestCopy

	targetHost, isLocal, err := o.resolveTargetNode(ctx, existing)
	if err != nil {
		return nil, err
	}
	if isLocal {
		drv, err := o.driverForInstance(ctx, existing)
		if err != nil {
			return nil, err
		}
		out, err := drv.Exec(ctx, existing, drvruntime.ExecRequest{Command: req.Command})
		if err != nil {
			return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
		}
		return &corev1.InstanceExec{
			TypeMeta:   metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceExec"},
			Metadata:   existing.Metadata,
			Command:    req.Command,
			Output:     out,
			ExecutedAt: time.Now().UTC(),
		}, nil
	}
	return o.proxyRemoteExec(ctx, targetHost, existing.Spec.NodeName, tenant, name, req)
}

func (o *operator) proxyRemoteExec(ctx context.Context, targetHost, nodeName, tenant, name string, req *corev1.InstanceExec) (*corev1.InstanceExec, error) {
	remoteURL := "http://" + targetHost + instanceSubresourcePath(tenant, name, "exec")
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal exec request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, remoteURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create proxy exec request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if tok := o.resolveNodeAgentToken(ctx, nodeName); tok != "" {
		httpReq.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to connect to node-agent on %s: %v", models.ErrBadGateway, targetHost, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, mapAgentProxyError(resp, targetHost, tenant, name, " exec")
	}
	var result corev1.InstanceExec
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode remote exec response: %w", err)
	}
	return &result, nil
}

func mapAgentProxyError(resp *http.Response, targetHost, tenant, name, prefix string) error {
	errMsg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: node-agent authentication failed on %s: %s", models.ErrUnauthorized, targetHost, string(errMsg))
	case http.StatusNotFound:
		return fmt.Errorf("%w: instance %s/%s not found on node %s: %s", models.ErrNotFound, tenant, name, targetHost, string(errMsg))
	default:
		return fmt.Errorf("%w: node-agent%s returned HTTP %d: %s", models.ErrBadGateway, prefix, resp.StatusCode, string(errMsg))
	}
}

// Files handles declarative file & backup intents. Direct file tickets and S3 download presigning
// are computed in-memory; backup, delete_backup, and restore persist declarative state and publish
// an MQ reconcile event so heavy quota/S3/agent transfers run asynchronously in ReconcileBackups.
func (o *operator) Files(ctx context.Context, tenant, name string, req *corev1.InstanceFiles) (*corev1.InstanceFiles, error) {
	if req == nil {
		req = &corev1.InstanceFiles{Action: corev1.FileActionList}
	}
	input := *req
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	if input.Action == "" {
		input.Action = corev1.FileActionList
	}
	existing, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if input.Metadata.UID != "" && input.Metadata.UID != existing.Metadata.UID {
		return nil, db.ErrConflict
	}
	maxBytes, err := resolveFileActionMaxBytes(existing, input)
	if err != nil {
		return nil, err
	}
	switch input.Action {
	case corev1.FileActionBackup:
		return o.requestBackupAction(ctx, existing, input, maxBytes)
	case corev1.FileActionUploadBackup:
		return o.handleUploadBackupAction(ctx, existing, input, maxBytes)
	case corev1.FileActionCompleteUploadBackup:
		return o.handleCompleteUploadBackupAction(ctx, existing, input)
	case corev1.FileActionDeleteBackup:
		return o.requestDeleteBackupAction(ctx, existing, input)
	case corev1.FileActionDownloadBackup:
		return o.handleDownloadBackupAction(ctx, existing, &input)
	case corev1.FileActionInspectBackup:
		return o.handleInspectBackupAction(ctx, existing, &input, maxBytes)
	case corev1.FileActionRestore:
		return o.requestRestoreBackupAction(ctx, existing, &input)
	default:
		return o.directFileTicket(ctx, existing, &input, maxBytes)
	}
}

func resolveFileActionMaxBytes(existing *corev1.Instance, spec corev1.InstanceFiles) (int64, error) {
	maxBytes := spec.MaxBytes
	if maxBytes < 0 || spec.RateLimitBytesPerSec < 0 {
		return 0, fmt.Errorf("%w: file budget and rate limit cannot be negative", models.ErrBadRequest)
	}
	if maxBytes == 0 {
		limits := existing.Spec.Resources.Limits
		if len(limits) == 0 {
			limits = existing.Spec.Resources.EffectiveRequests()
		}
		maxBytes = limits.StorageBytes()
		if maxBytes <= 0 {
			maxBytes = 20 << 30
		}
	}
	return maxBytes, nil
}

func (o *operator) fileExecutionClaims(ctx context.Context, inst *corev1.Instance) (volume.FileTicketClaims, error) {
	c := inst.Status.ResourceControl
	if inst.Metadata.DeletionTimestamp != nil || c == nil || c.Stage != string(StageActive) || c.CurrentAttempt == nil || c.CurrentAttempt.Phase != AttemptBound || c.ApprovedRevision < 1 {
		return volume.FileTicketClaims{}, fmt.Errorf("%w: files require an active bound instance", models.ErrConflict)
	}
	a := c.CurrentAttempt
	if a.ID == "" || a.NodeUID == "" || a.Epoch < 1 || a.NodeName == "" || a.NodeName != inst.Spec.NodeName || inst.Metadata.UID == "" {
		return volume.FileTicketClaims{}, fmt.Errorf("%w: file execution binding is incomplete", models.ErrConflict)
	}
	node, err := o.nodeDB.Get(ctx, a.NodeName)
	if err != nil {
		return volume.FileTicketClaims{}, err
	}
	if node.Metadata.UID != a.NodeUID || node.Metadata.DeletionTimestamp != nil {
		return volume.FileTicketClaims{}, db.ErrConflict
	}
	return volume.FileTicketClaims{
		InstanceUID: inst.Metadata.UID, Tenant: inst.Metadata.Tenant, Instance: inst.Metadata.Name,
		NodeName: a.NodeName, NodeUID: a.NodeUID, AttemptID: a.ID, Epoch: a.Epoch, ApprovedRevision: c.ApprovedRevision,
	}, nil
}

func (o *operator) transferFilesStreaming(ctx context.Context, inst *corev1.Instance, req *corev1.InstanceFiles, expires time.Time, onEvent func(corev1.InstanceTaskStreamEvent)) (*corev1.InstanceFiles, error) {
	httpReq, err := o.buildTransferFilesRequest(ctx, inst, req, expires)
	if err != nil {
		return nil, err
	}
	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: node-agent transfer acknowledgement unavailable: %v", models.ErrBadGateway, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, mapTransferHTTPError(resp)
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/x-ndjson") {
		return consumeTransferNDJSON(resp.Body, onEvent)
	}
	var result corev1.InstanceFiles
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("%w: invalid node-agent file response", models.ErrBadGateway)
	}
	result.PresignedURL = ""
	return &result, nil
}

func (o *operator) buildTransferFilesRequest(ctx context.Context, inst *corev1.Instance, req *corev1.InstanceFiles, expires time.Time) (*http.Request, error) {
	claims, err := o.fileExecutionClaims(ctx, inst)
	if err != nil {
		return nil, err
	}
	claims.Action, claims.Path, claims.TargetPath = req.Action, req.Path, req.TargetPath
	claims.SourcePath = req.SourcePath
	claims.ReplaceTarget = req.ReplaceTarget != nil && *req.ReplaceTarget
	claims.PresignedURL, claims.MaxBytes, claims.RateLimit, claims.ExpiresAt = req.PresignedURL, req.MaxBytes, req.RateLimitBytesPerSec, expires.Unix()
	secret := o.resolveNodeAgentToken(ctx, claims.NodeName)
	ticket, err := volume.SignFileTicket(secret, claims)
	if err != nil {
		return nil, fmt.Errorf("sign file ticket: %w", err)
	}
	targetHost, _, err := o.resolveTargetNode(ctx, inst)
	if err != nil {
		return nil, err
	}
	if targetHost == "" {
		targetHost = net.JoinHostPort("127.0.0.1", strconv.Itoa(o.agentPort))
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	remoteURL := "http://" + targetHost + instanceSubresourcePath(inst.Metadata.Tenant, inst.Metadata.Name, "files") + "?ticket=" + url.QueryEscape(ticket)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, remoteURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/x-ndjson, application/json")
	if secret != "" {
		httpReq.Header.Set("Authorization", "Bearer "+secret)
	}
	return httpReq, nil
}

func mapTransferHTTPError(resp *http.Response) error {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("%w: node-agent rejected file execution binding", models.ErrConflict)
	}
	return fmt.Errorf("%w: node-agent files returned HTTP %d", models.ErrBadGateway, resp.StatusCode)
}

func consumeTransferNDJSON(body io.Reader, onEvent func(corev1.InstanceTaskStreamEvent)) (*corev1.InstanceFiles, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 16*1024), 1<<20)
	var finalResult *corev1.InstanceFiles
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev corev1.InstanceTaskStreamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if onEvent != nil {
			onEvent(ev)
		}
		if ev.Type == corev1.TaskStreamEventError {
			return nil, fmt.Errorf("%w: %s", models.ErrBadGateway, ev.Message)
		}
		if ev.Type == corev1.TaskStreamEventResult && ev.Result != nil {
			finalResult = ev.Result
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: transfer stream interrupted: %v", models.ErrBadGateway, err)
	}
	if finalResult == nil {
		return nil, fmt.Errorf("%w: node-agent transfer stream closed before completion", models.ErrBadGateway)
	}
	finalResult.PresignedURL = ""
	return finalResult, nil
}

func (o *operator) directFileTicket(ctx context.Context, inst *corev1.Instance, req *corev1.InstanceFiles, maxBytes int64) (*corev1.InstanceFiles, error) {
	action := req.Action
	switch action {
	case corev1.FileActionList, corev1.FileActionRead, corev1.FileActionWrite, corev1.FileActionDelete, corev1.FileActionMkdir, corev1.FileActionRename:
	default:
		return nil, fmt.Errorf("%w: unsupported file action %q", models.ErrBadRequest, action)
	}
	if err := validateDirectFilePaths(req); err != nil {
		return nil, err
	}
	claims, err := o.fileExecutionClaims(ctx, inst)
	if err != nil {
		return nil, err
	}
	directHost, err := o.resolveDirectNodeHost(ctx, claims.NodeName)
	if err != nil {
		return nil, err
	}
	expires := time.Now().UTC().Add(15 * time.Minute)
	claims.Action, claims.Path, claims.TargetPath = action, req.Path, req.TargetPath
	claims.MaxBytes, claims.RateLimit, claims.ExpiresAt = maxBytes, req.RateLimitBytesPerSec, expires.Unix()
	secret := o.resolveNodeAgentToken(ctx, claims.NodeName)
	ticket, err := volume.SignFileTicket(secret, claims)
	if err != nil {
		return nil, err
	}
	q := url.Values{"ticket": {ticket}}
	if req.Path != "" {
		q.Set("path", req.Path)
	}
	return &corev1.InstanceFiles{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceFiles"}, Metadata: inst.Metadata,
		Action: action, Path: req.Path, TargetPath: req.TargetPath, MaxBytes: maxBytes, RateLimitBytesPerSec: req.RateLimitBytesPerSec,
		Ticket: ticket, DirectURL: "http://" + directHost + instanceSubresourcePath(inst.Metadata.Tenant, inst.Metadata.Name, "files") + "?" + q.Encode(), ExpiresAt: &expires,
	}, nil
}

func validateDirectFilePaths(req *corev1.InstanceFiles) error {
	allowRoot := req.Action == corev1.FileActionList || (req.Action == corev1.FileActionWrite && strings.TrimSpace(req.Path) == "/")
	rel, err := metav1.NormalizeRelativeFilePath(req.Path, allowRoot)
	if err != nil {
		return fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	if rel != "." && volume.IsReservedMetadataPath(rel) {
		return fmt.Errorf("%w: reserved internal workspace path is not accessible", models.ErrBadRequest)
	}
	if req.Action == corev1.FileActionRename && strings.TrimSpace(req.TargetPath) != "" {
		targetRel, err := metav1.NormalizeRelativeFilePath(req.TargetPath, false)
		if err != nil {
			return fmt.Errorf("%w: %v", models.ErrBadRequest, err)
		}
		if volume.IsReservedMetadataPath(targetRel) {
			return fmt.Errorf("%w: reserved internal workspace path is not accessible", models.ErrBadRequest)
		}
	}
	return nil
}

func (o *operator) resolveDirectNodeHost(ctx context.Context, nodeName string) (string, error) {
	node, err := o.nodeDB.Get(ctx, nodeName)
	if err != nil {
		return "", err
	}
	directHost := resolveNodePublicOrInternalIP(node)
	if directHost == "" {
		directHost = "127.0.0.1"
	}
	if _, _, err := net.SplitHostPort(directHost); err != nil {
		directHost = net.JoinHostPort(directHost, strconv.Itoa(o.agentPort))
	}
	return directHost, nil
}
