package server

import "errors"

var ErrReconcileNotImplemented = errors.New("server reconciliation is not implemented yet")
var ErrWorkloadNotFound = errors.New("workload not found")
var ErrUpdateNotSupported = errors.New("workload resource update not supported")
