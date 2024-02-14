package common

import "errors"

var (
	ErrInvalidState                = errors.New("The operation is not allowed")
	ErrRequirementParameterMissing = errors.New("Missing required parameter")
	ErrRequirementParameterInvalid = errors.New("Invalid value for required parameter")
	ErrDuplicateMapEntry           = errors.New("Duplicate name detected")
)

const (
	// WorkerIdLogLabel is the key used in structure logs for worker id.
	WorkerIdLogLabel = "worker_id"
	// WorkerPoolLogLabel is the key used in structure logs for worker pool name.
	WorkerPoolLogLabel = "pool"
)
