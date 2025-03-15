package nodejs

import "errors"

// Package-level error definitions for common error conditions
var (
	// ErrTimeout is returned when a NodeJS operation exceeds its allowed time limit
	ErrTimeout = errors.New("nodejs: timeout reached")

	// ErrDeadProcess is returned when attempting to interact with a NodeJS process
	// that has already terminated
	ErrDeadProcess = errors.New("nodejs: process has died")
)
