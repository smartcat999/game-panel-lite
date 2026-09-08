package workload

// NodeSession orders heartbeats only; it is not a workload execution grant.
type NodeSession struct {
	Epoch int64 `json:"epoch"`
}

type NodeHeartbeat struct {
	SessionEpoch int64  `json:"sessionEpoch"`
	Sequence     int64  `json:"sequence"`
	Architecture string `json:"architecture"`
	RuntimeReady bool   `json:"runtimeReady"`
}
