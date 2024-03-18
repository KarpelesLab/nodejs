package nodejs

import "errors"

var (
	ErrTimeout     = errors.New("nodejs: timeout reached")
	ErrDeadProcess = errors.New("nodejs: process has died")
)
