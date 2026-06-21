package server

import "errors"

var ErrReconcileNotImplemented = errors.New("server reconciliation is not implemented yet")
var ErrWorkloadNotFound = errors.New("workload not found")
