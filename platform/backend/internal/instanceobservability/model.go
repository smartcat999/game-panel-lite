package instanceobservability

import "time"

type LogEntry struct {
	ID                string    `json:"id"`
	LogicalInstanceID string    `json:"logicalInstanceId"`
	RuntimeAttemptID  string    `json:"runtimeAttemptId,omitempty"`
	Stream            string    `json:"stream"`
	Message           string    `json:"message"`
	ObservedAt        time.Time `json:"observedAt"`
}

type MetricSample struct {
	ID                string     `json:"id"`
	LogicalInstanceID string     `json:"logicalInstanceId"`
	RuntimeAttemptID  string     `json:"runtimeAttemptId,omitempty"`
	Metric            string     `json:"metric"`
	Value             float64    `json:"value"`
	Unit              string     `json:"unit"`
	Source            string     `json:"source"`
	Confidence        *float64   `json:"confidence,omitempty"`
	FreshUntil        *time.Time `json:"freshUntil,omitempty"`
	SampledAt         time.Time  `json:"sampledAt"`
}

type Observation struct {
	MessageID            string
	WorkspaceID          string
	LogicalInstanceID    string
	RegionID             string
	RegionalDeploymentID string
	RuntimeAttemptID     string
	Sequence             int64
	Logs                 []LogEntry
	Metrics              []MetricSample
	ObservedAt           time.Time
}
