package backup

import (
	"errors"
	"strings"
)

// ErrRequestUnavailable hides whether an old or foreign request exists.
var ErrRequestUnavailable = errors.New("backup request unavailable")

var ErrInvalidRequest = errors.New("invalid backup request")

type Request struct {
	OrganizationID string `json:"organizationId"`
	ServerID       string `json:"serverId"`
	Scope          string `json:"scope"`
	IdempotencyKey string `json:"-"`
}

func (r Request) Validate() error {
	if !backupID(r.OrganizationID) || !backupID(r.ServerID) || !backupID(r.IdempotencyKey) || (r.Scope != "instance" && r.Scope != "world") {
		return ErrInvalidRequest
	}
	return nil
}

// Requested is global intent, not proof that a Node is stopped or a snapshot
// is consistent. Region must revalidate execution authority before preparing it.
type Requested struct {
	SchemaVersion  int    `json:"schemaVersion"`
	EventID        string `json:"eventId"`
	OperationID    string `json:"operationId"`
	BackupID       string `json:"backupId"`
	OrganizationID string `json:"organizationId"`
	ServerID       string `json:"serverId"`
	RegionID       string `json:"regionId"`
	RevisionID     string `json:"revisionId"`
	SpecGeneration int64  `json:"specGeneration"`
	IntentVersion  int64  `json:"intentVersion"`
	PlacementEpoch int64  `json:"placementEpoch"`
	Scope          string `json:"scope"`
}

func (e Requested) Validate() error {
	if e.SchemaVersion != 1 || e.SpecGeneration < 1 || e.IntentVersion < 1 || e.PlacementEpoch < 1 || (e.Scope != "instance" && e.Scope != "world") {
		return ErrInvalidRequest
	}
	for _, id := range []string{e.EventID, e.OperationID, e.BackupID, e.OrganizationID, e.ServerID, e.RegionID, e.RevisionID} {
		if !backupID(id) {
			return ErrInvalidRequest
		}
	}
	return nil
}

type Task struct {
	Request Requested `json:"request"`
	Status  string    `json:"status"`
}

func backupID(id string) bool {
	return id != "" && len(id) <= 128 && id == strings.TrimSpace(id) && !strings.ContainsAny(id, "\x00\r\n")
}
