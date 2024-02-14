package worker

import (
	"errors"
	"math"
)

const (
	// DefaultSignalTimeoutSec is the default time a worker has to respond to a signal before SIGTERM is issued.
	DefaultSignalTimeoutSec = 10

	// DefaultMaxRestarts is the default times a worker is allowed to restart before terminating the system.
	DefaultMaxRestarts = math.MaxInt
)

var (
	ErrTooManyRestarts    = errors.New("Worker Pool too many restarts")
	errExecutableNotFound = errors.New("Executable doesn't exist")
)
