package supervisor

import (
	"errors"
)

var (
	errDuplicateWorkerName = errors.New("Duplicate worker name pattern detected")
)
