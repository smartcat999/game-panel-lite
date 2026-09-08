package regional

import (
	"errors"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

var ErrNodeUnavailable = errors.New("regional node unavailable")
var ErrNodeHeartbeatStale = errors.New("node heartbeat session or sequence is stale")

type NodeSession = workload.NodeSession

type NodeHeartbeat = workload.NodeHeartbeat
