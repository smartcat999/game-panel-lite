package regional

import "errors"

var ErrNodeUnavailable = errors.New("regional node unavailable")
var ErrNodeHeartbeatStale = errors.New("node heartbeat session or sequence is stale")

type NodeSession struct {
	Epoch int64 `json:"epoch"`
}

// Heartbeats convey runtime observations only. Identity comes from the verified
// transport. Epoch is issued by Region; Sequence increases within that session.
type NodeHeartbeat struct {
	SessionEpoch int64  `json:"sessionEpoch"`
	Sequence     int64  `json:"sequence"`
	Architecture string `json:"architecture"`
	RuntimeReady bool   `json:"runtimeReady"`
}
