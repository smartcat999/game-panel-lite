package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
	"gorm.io/gorm"
)

// ResolveArtifactForNode authorizes a read against the current assignment and
// instance, not a mod ID supplied independently by the node. It does not grant a
// lease: callers recheck after opening a file and in-flight revocation is separate.
func (s *Store) ResolveArtifactForNode(ctx context.Context, nodeID, uid string, generation int, artifactID string) (domain.ModFile, workload.Artifact, error) {
	var item domain.ModFile
	var ref workload.Artifact
	var assignment domain.WorkloadAssignment
	if nodeID == "" || uid == "" || generation <= 0 || artifactID == "" {
		return item, ref, ErrNotFound
	}
	err := s.db.WithContext(ctx).Where("uid = ? AND node_id = ? AND generation = ? AND desired_state = ? AND deletion_timestamp IS NULL", uid, nodeID, generation, domain.DesiredRunning).Take(&assignment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return item, ref, ErrNotFound
	}
	if err != nil {
		return item, ref, err
	}
	if err := workload.ValidateArtifacts(assignment.Spec.Options); err != nil {
		return item, ref, ErrInvalidModLibrary
	}
	found := false
	for _, candidate := range assignment.Spec.Options.Artifacts {
		if candidate.ID != artifactID {
			continue
		}
		if found && (candidate.SHA256 != ref.SHA256 || candidate.SizeBytes != ref.SizeBytes) {
			return item, ref, ErrInvalidModLibrary
		}
		if !found {
			ref = candidate
			found = true
		}
	}
	if !found {
		return item, ref, ErrNotFound
	}
	target, err := s.GetGameServer(ctx, assignment.ServerID)
	if err != nil {
		return item, ref, err
	}
	if target.OrganizationID == "" || target.NodeID != nodeID || target.Spec.Generation != generation || target.Spec.DesiredState != domain.DesiredRunning {
		return item, ref, ErrNotFound
	}
	err = s.db.WithContext(ctx).Where("id = ? AND instance_id = ? AND organization_id = ? AND provider_key = ? AND source = ? AND EXISTS (SELECT 1 FROM organizations WHERE organizations.id = mod_files.organization_id)", artifactID, "unassigned", target.OrganizationID, target.ProviderKey, "upload").Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return item, ref, ErrNotFound
	}
	if err != nil {
		return item, ref, err
	}
	if item.ContentHash != ref.SHA256 || item.SizeBytes != ref.SizeBytes {
		return domain.ModFile{}, ref, ErrInvalidModLibrary
	}
	if err := s.CheckModTarget(ctx, target); err != nil {
		return domain.ModFile{}, ref, err
	}
	spec, err := json.Marshal(assignment.Spec)
	if err != nil {
		return domain.ModFile{}, ref, err
	}
	var count int64
	err = s.db.WithContext(ctx).Model(&domain.WorkloadAssignment{}).Where("uid = ? AND node_id = ? AND server_id = ? AND generation = ? AND desired_state = ? AND deletion_timestamp IS NULL AND spec = ?", uid, nodeID, target.ID, generation, domain.DesiredRunning, string(spec)).Count(&count).Error
	if err != nil {
		return domain.ModFile{}, ref, err
	}
	if count != 1 {
		return domain.ModFile{}, ref, ErrReconciliationSuperseded
	}
	return item, ref, nil
}
