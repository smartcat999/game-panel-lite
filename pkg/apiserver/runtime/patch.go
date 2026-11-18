package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	jsonpatch "gopkg.in/evanphx/json-patch.v4"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// ApplyMergePatch applies an RFC 7396 JSON Merge Patch (via gopkg.in/evanphx/json-patch.v4)
// from r.Body onto current, preserving immutable identity fields (Name, Tenant, UID, CreationTimestamp, ResourceVersion).
func ApplyMergePatch[T metav1.Object](r *http.Request, current T) (T, error) {
	var zero T
	if r == nil || r.Body == nil {
		return zero, errors.New("empty merge patch request body")
	}

	patchBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return zero, fmt.Errorf("read merge patch body: %w", err)
	}
	if len(bytes.TrimSpace(patchBytes)) == 0 {
		return zero, errors.New("empty merge patch body")
	}

	var patchMap map[string]any
	if err := json.Unmarshal(patchBytes, &patchMap); err != nil || patchMap == nil {
		return zero, errors.New("invalid merge patch payload: must be a JSON object")
	}

	curBytes, err := json.Marshal(current)
	if err != nil {
		return zero, fmt.Errorf("marshal current object: %w", err)
	}

	mergedBytes, err := jsonpatch.MergePatch(curBytes, patchBytes)
	if err != nil {
		return zero, fmt.Errorf("invalid merge patch payload: %w", err)
	}

	var out T
	if err := json.Unmarshal(mergedBytes, &out); err != nil {
		return zero, fmt.Errorf("unmarshal patched object: %w", err)
	}

	srcMeta := current.GetObjectMeta()
	dstMeta := out.GetObjectMeta()
	dstMeta.Name = srcMeta.Name
	dstMeta.Tenant = srcMeta.Tenant
	dstMeta.UID = srcMeta.UID
	dstMeta.CreationTimestamp = srcMeta.CreationTimestamp
	if dstMeta.ResourceVersion == "" {
		dstMeta.ResourceVersion = srcMeta.ResourceVersion
	}

	return out, nil
}
