package client

import (
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// ApplyAction indicates the outcome of a declarative Apply operation.
type ApplyAction string

const (
	ActionCreated    ApplyAction = "created"
	ActionConfigured ApplyAction = "configured"
	ActionUnchanged  ApplyAction = "unchanged"
)

// ApplyResult represents the result of applying a single declarative object.
type ApplyResult struct {
	Action   ApplyAction
	Resource *ResourceInfo
	Object   metav1.Object
}

// ListOptions specifies pagination, filtering, and tenancy options for list requests.
type ListOptions = rest.ListOptions

// LogOptions specifies log streaming parameters.
type LogOptions = rest.LogOptions
